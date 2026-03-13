package build

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	internalexec "github.com/you/wailsrel/internal/exec"
)

func RequiredTools(target Target) []string {
	tools := []string{"wails3"}
	switch target.OS {
	case "darwin":
		if slices.Contains(target.OutputFormats, "dmg") {
			tools = append(tools, "hdiutil")
		}
	case "windows":
		if slices.Contains(target.OutputFormats, "nsis") {
			tools = append(tools, "makensis")
		}
	case "linux":
		if slices.Contains(target.OutputFormats, "appimage") {
			tools = append(tools, "appimagetool")
		}
		if slices.Contains(target.OutputFormats, "deb") {
			tools = append(tools, "dpkg-deb")
		}
		if slices.Contains(target.OutputFormats, "rpm") {
			tools = append(tools, "rpmbuild")
		}
	}
	return tools
}

func (b *WailsBuilder) Available(ctx context.Context, target Target) error {
	tools := RequiredTools(target)

	var missing []string
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required tools for %s/%s: %v", target.OS, target.Arch, missing)
	}

	signer, err := b.newSigner(target.Sign)
	if err != nil {
		return err
	}
	if err := signer.Available(ctx); err != nil {
		return fmt.Errorf("signing unavailable for %s/%s: %w", target.OS, target.Arch, err)
	}

	return nil
}

func (b *WailsBuilder) Build(ctx context.Context, target Target) (*BuildResult, error) {
	start := time.Now()

	if err := os.MkdirAll(b.outputDir, 0o755); err != nil {
		return nil, err
	}

	var (
		artifacts []Artifact
		err       error
	)

	switch target.OS {
	case "darwin":
		artifacts, err = b.buildDarwin(ctx, target)
	case "windows":
		artifacts, err = b.buildWindows(ctx, target)
	case "linux":
		artifacts, err = b.buildLinux(ctx, target)
	default:
		err = fmt.Errorf("unsupported target OS %q", target.OS)
	}
	if err != nil {
		return nil, err
	}

	return &BuildResult{
		Artifacts: artifacts,
		Duration:  time.Since(start),
	}, nil
}

func (b *WailsBuilder) run(ctx context.Context, name string, args ...string) error {
	_, err := b.runner.Run(ctx, name, args, internalexec.Options{
		Dir:     b.projectDir,
		Timeout: b.timeout,
		Logger:  b.logger,
	})
	if err != nil {
		return fmt.Errorf("%s %v: %w", name, args, err)
	}
	return nil
}

func (b *WailsBuilder) wailsBuild(ctx context.Context, target Target) error {
	return b.run(ctx, "wails3", "task", "build", "GOOS="+target.OS, "GOARCH="+target.Arch)
}

func (b *WailsBuilder) wailsPackage(ctx context.Context, target Target) error {
	return b.run(ctx, "wails3", "package", "GOOS="+target.OS, "GOARCH="+target.Arch)
}

func (b *WailsBuilder) stageArtifact(target Target, format, sourcePath string, metadata map[string]string) (Artifact, error) {
	relative := filepath.Join(target.OS, target.Arch, filepath.Base(sourcePath))
	destination := filepath.Join(b.outputDir, relative)

	if err := os.RemoveAll(destination); err != nil {
		return Artifact{}, err
	}
	if err := copyPath(sourcePath, destination); err != nil {
		return Artifact{}, err
	}

	checksum, err := ComputeChecksum(destination)
	if err != nil {
		return Artifact{}, err
	}
	if err := writeChecksumFile(destination, checksum); err != nil {
		return Artifact{}, err
	}

	size, err := pathSize(destination)
	if err != nil {
		return Artifact{}, err
	}

	return Artifact{
		Path:     normalizeArtifactPath(relative),
		OS:       target.OS,
		Arch:     target.Arch,
		Format:   format,
		Checksum: "sha256:" + checksum,
		Size:     size,
		Metadata: metadata,
	}, nil
}

func (b *WailsBuilder) stageZip(target Target, sourcePath string) (Artifact, error) {
	name := fmt.Sprintf("%s-%s-%s.zip", b.appName, target.OS, target.Arch)
	tempPath := filepath.Join(b.projectDir, ".wailsrel", "tmp", name)
	if err := os.MkdirAll(filepath.Dir(tempPath), 0o755); err != nil {
		return Artifact{}, err
	}
	if err := zipPath(sourcePath, tempPath); err != nil {
		return Artifact{}, err
	}
	return b.stageArtifact(target, "zip", tempPath, nil)
}

func requirePath(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("expected build artifact %s to exist", path)
		}
		return err
	}
	return nil
}
