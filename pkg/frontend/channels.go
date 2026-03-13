package frontend

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type BundleManager struct {
	AppID        string
	NativeCompat string
	OverrideRoot string
}

func (bm *BundleManager) ActiveChannel() string {
	data, err := os.ReadFile(bm.channelMarkerPath())
	if err != nil {
		return "stable"
	}
	channel := strings.TrimSpace(string(data))
	if channel == "" {
		return "stable"
	}
	return channel
}

func (bm *BundleManager) SetChannel(channel string) error {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return fmt.Errorf("channel is required")
	}
	if err := os.MkdirAll(bm.rootDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(bm.channelMarkerPath(), []byte(channel+"\n"), 0o644)
}

func (bm *BundleManager) Install(ctx context.Context, channel string, bundle io.Reader) error {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return fmt.Errorf("channel is required")
	}

	if err := os.MkdirAll(bm.rootDir(), 0o755); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(bm.rootDir(), "bundle-*.zip")
	if err != nil {
		return err
	}
	tempZipPath := tempFile.Name()
	defer os.Remove(tempZipPath)

	if _, err := io.Copy(tempFile, bundle); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	manifest, err := readBundleManifestFromZip(tempZipPath)
	if err != nil {
		return err
	}
	if manifest.Channel != "" && manifest.Channel != channel {
		return fmt.Errorf("bundle channel %q does not match requested channel %q", manifest.Channel, channel)
	}
	if err := CheckCompat(*manifest, bm.NativeCompat); err != nil {
		return err
	}

	stagingDir, err := os.MkdirTemp(bm.rootDir(), "channel-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stagingDir)

	if err := extractBundleArchive(tempZipPath, stagingDir); err != nil {
		return err
	}
	if err := writeBundleManifest(filepath.Join(stagingDir, "bundle.json"), *manifest); err != nil {
		return err
	}

	finalDir := bm.channelDir(channel)
	backupDir := finalDir + ".old"
	_ = os.RemoveAll(backupDir)
	if _, err := os.Stat(finalDir); err == nil {
		if err := os.Rename(finalDir, backupDir); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		_ = os.Rename(backupDir, finalDir)
		return err
	}
	if err := os.Rename(stagingDir, finalDir); err != nil {
		_ = os.Rename(backupDir, finalDir)
		return err
	}
	_ = os.RemoveAll(backupDir)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (bm *BundleManager) LoadActive() (fs.FS, *BundleManifest, error) {
	channel := bm.ActiveChannel()
	manifest, err := loadBundleManifest(filepath.Join(bm.channelDir(channel), "bundle.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if err := CheckCompat(*manifest, bm.NativeCompat); err != nil {
		return nil, nil, nil
	}
	return os.DirFS(bm.channelDir(channel)), manifest, nil
}

func (bm *BundleManager) ListInstalled() ([]BundleManifest, error) {
	entries, err := os.ReadDir(bm.channelsRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	manifests := make([]BundleManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := loadBundleManifest(filepath.Join(bm.channelsRoot(), entry.Name(), "bundle.json"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		manifests = append(manifests, *manifest)
	}

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].Channel < manifests[j].Channel
	})
	return manifests, nil
}

func (bm *BundleManager) Cleanup() error {
	entries, err := os.ReadDir(bm.channelsRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		channel := entry.Name()
		manifest, err := loadBundleManifest(filepath.Join(bm.channelsRoot(), channel, "bundle.json"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := CheckCompat(*manifest, bm.NativeCompat); err != nil {
			if removeErr := os.RemoveAll(filepath.Join(bm.channelsRoot(), channel)); removeErr != nil {
				return removeErr
			}
		}
	}

	return nil
}

func (bm *BundleManager) rootDir() string {
	if strings.TrimSpace(bm.OverrideRoot) != "" {
		return bm.OverrideRoot
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", ".wailsrel", safePathPart(bm.AppID))
	}
	return filepath.Join(base, "wailsrel", safePathPart(bm.AppID), "frontend")
}

func (bm *BundleManager) channelsRoot() string {
	return filepath.Join(bm.rootDir(), "channels")
}

func (bm *BundleManager) channelDir(channel string) string {
	return filepath.Join(bm.channelsRoot(), safePathPart(channel))
}

func (bm *BundleManager) channelMarkerPath() string {
	return filepath.Join(bm.rootDir(), "frontend-channel")
}

func safePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}

	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return replacer.Replace(value)
}
