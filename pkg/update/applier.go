package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/Eriyc/wailsrel/pkg/delta"
	"github.com/Eriyc/wailsrel/pkg/frontend"
)

type Applier interface {
	ApplyNative(ctx context.Context, info *UpdateInfo, progress ProgressFunc) error
	ApplyFrontend(ctx context.Context, info *FrontendUpdateInfo, progress ProgressFunc) error
}

type ApplierOptions struct {
	Client          *http.Client
	FrontendManager *frontend.BundleManager
	TargetPath      string
	TempDir         string
}

type DefaultApplier struct {
	client          *http.Client
	frontendManager *frontend.BundleManager
	targetPath      string
	tempDir         string
	deltaApply      func(oldPath, patchPath, outPath string) error
	verifyInstalled func(context.Context, string) error
}

func NewApplier(opts ApplierOptions) *DefaultApplier {
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}
	tempDir := strings.TrimSpace(opts.TempDir)
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &DefaultApplier{
		client:          client,
		frontendManager: opts.FrontendManager,
		targetPath:      opts.TargetPath,
		tempDir:         tempDir,
		deltaApply: func(oldPath, patchPath, outPath string) error {
			result, err := delta.Apply(oldPath, patchPath, outPath)
			if err != nil {
				return err
			}
			if result == nil {
				return fmt.Errorf("delta apply returned no result")
			}
			return nil
		},
	}
}

func (a *DefaultApplier) ApplyNative(ctx context.Context, info *UpdateInfo, progress ProgressFunc) error {
	if info == nil {
		return fmt.Errorf("update info is required")
	}
	if strings.TrimSpace(a.targetPath) == "" {
		return fmt.Errorf("target path is required")
	}
	if err := os.MkdirAll(a.tempDir, 0o755); err != nil {
		return err
	}

	if info.DeltaURL != "" {
		patchPath := filepath.Join(a.tempDir, "update.patch")
		if err := a.downloadToFile(ctx, info.DeltaURL, patchPath, info.DeltaHash, progress); err == nil {
			candidatePath := filepath.Join(a.tempDir, "candidate.bin")
			if err := a.deltaApply(a.targetPath, patchPath, candidatePath); err == nil {
				if err := verifyChecksum(candidatePath, info.ArtifactHash); err == nil {
					return a.replaceWithRollback(ctx, candidatePath)
				}
			}
		}
	}

	downloadPath := filepath.Join(a.tempDir, "full-download.bin")
	if err := a.downloadToFile(ctx, info.ArtifactURL, downloadPath, info.ArtifactHash, progress); err != nil {
		return err
	}
	return a.replaceWithRollback(ctx, downloadPath)
}

func (a *DefaultApplier) ApplyFrontend(ctx context.Context, info *FrontendUpdateInfo, progress ProgressFunc) error {
	if info == nil {
		return fmt.Errorf("frontend update info is required")
	}
	if a.frontendManager == nil {
		return fmt.Errorf("frontend manager is required")
	}
	if err := os.MkdirAll(a.tempDir, 0o755); err != nil {
		return err
	}

	downloadPath := filepath.Join(a.tempDir, "frontend-bundle.zip")
	if err := a.downloadToFile(ctx, info.URL, downloadPath, info.Hash, progress); err != nil {
		return err
	}

	file, err := os.Open(downloadPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return a.frontendManager.Install(ctx, info.Channel, file)
}

func (a *DefaultApplier) downloadToFile(ctx context.Context, sourceURL, destinationPath, expectedChecksum string, progress ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}
	partPath := destinationPath + ".part"

	offset := int64(0)
	if info, err := os.Stat(partPath); err == nil {
		offset = info.Size()
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: unexpected status %s", sourceURL, resp.Status)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if offset > 0 && resp.StatusCode == http.StatusPartialContent {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		offset = 0
	}

	out, err := os.OpenFile(partPath, flags, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	total := resp.ContentLength
	if total >= 0 {
		total += offset
	}
	downloaded := offset
	if progress != nil && downloaded > 0 {
		progress(downloaded, total)
	}
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			written, writeErr := out.Write(buffer[:n])
			downloaded += int64(written)
			if progress != nil {
				progress(downloaded, total)
			}
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	if progress != nil && total >= 0 && downloaded != total {
		progress(downloaded, downloaded)
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := verifyChecksum(partPath, expectedChecksum); err != nil {
		return err
	}
	return os.Rename(partPath, destinationPath)
}

func verifyChecksum(path, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}

	checksum, err := build.ComputeChecksum(path)
	if err != nil {
		return err
	}
	if withSHA256Prefix(checksum) != withSHA256Prefix(expected) {
		return fmt.Errorf("checksum mismatch for %s", path)
	}
	return nil
}

func (a *DefaultApplier) replaceWithRollback(ctx context.Context, newPath string) error {
	targetInfo, err := os.Stat(a.targetPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	backupPath := a.targetPath + ".bak"
	_ = os.Remove(backupPath)
	if err == nil {
		if err := os.Rename(a.targetPath, backupPath); err != nil {
			return err
		}
	}

	if targetInfo != nil {
		if err := os.Chmod(newPath, targetInfo.Mode().Perm()); err != nil {
			_ = os.Rename(backupPath, a.targetPath)
			return err
		}
	}

	if err := replaceTargetForRuntime(a.targetPath, newPath); err != nil {
		_ = os.Rename(backupPath, a.targetPath)
		return err
	}

	if a.verifyInstalled != nil {
		if err := a.verifyInstalled(ctx, a.targetPath); err != nil {
			_ = os.RemoveAll(a.targetPath)
			if restoreErr := os.Rename(backupPath, a.targetPath); restoreErr != nil {
				return fmt.Errorf("verify installed: %w (rollback failed: %v)", err, restoreErr)
			}
			return err
		}
	}

	_ = os.Remove(backupPath)
	return nil
}

func replaceTargetForRuntime(targetPath, newPath string) error {
	switch runtime.GOOS {
	case "windows":
		return replaceTargetWindows(targetPath, newPath)
	case "darwin":
		return replaceTargetDarwin(targetPath, newPath)
	default:
		return replaceTargetLinux(targetPath, newPath)
	}
}

func replaceTargetLinux(targetPath, newPath string) error {
	return os.Rename(newPath, targetPath)
}

func replaceTargetDarwin(targetPath, newPath string) error {
	return os.Rename(newPath, targetPath)
}

func replaceTargetWindows(targetPath, newPath string) error {
	return os.Rename(newPath, targetPath)
}
