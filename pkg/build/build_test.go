package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/you/wailsrel/pkg/config"
)

func TestExpandMatrix(t *testing.T) {
	matrix := ExpandMatrix([]config.TargetConfig{
		{
			OS:            "darwin",
			Arch:          []string{"amd64", "arm64"},
			OutputFormats: []string{"app", "dmg"},
		},
		{
			OS:            "windows",
			Arch:          []string{"amd64"},
			OutputFormats: []string{"exe"},
		},
	})

	if len(matrix) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(matrix))
	}
	if matrix[1].OS != "darwin" || matrix[1].Arch != "arm64" {
		t.Fatalf("unexpected second target: %+v", matrix[1])
	}
}

func TestComputeChecksumForDirectoryIsStable(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "My.app"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "My.app", "Contents.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	first, err := ComputeChecksum(filepath.Join(root, "My.app"))
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}
	second, err := ComputeChecksum(filepath.Join(root, "My.app"))
	if err != nil {
		t.Fatalf("checksum second: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable checksum, got %q != %q", first, second)
	}
}

func TestBuildLinuxStagesArtifacts(t *testing.T) {
	projectDir := t.TempDir()
	outputDir := filepath.Join(projectDir, "dist")

	if err := os.MkdirAll(filepath.Join(projectDir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	pathEntries := []string{filepath.Join(projectDir, "tools")}
	if err := os.MkdirAll(pathEntries[0], 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}

	writeTool(t, filepath.Join(pathEntries[0], "wails3"), linuxToolScript())
	writeTool(t, filepath.Join(pathEntries[0], "appimagetool"), "#!/bin/sh\nexit 0\n")
	writeTool(t, filepath.Join(pathEntries[0], "dpkg-deb"), "#!/bin/sh\nexit 0\n")

	t.Setenv("PATH", pathEntries[0]+string(os.PathListSeparator)+os.Getenv("PATH"))

	builder := NewBuilder(Options{
		ProjectDir: projectDir,
		OutputDir:  outputDir,
		AppName:    "MyApp",
		Timeout:    5 * time.Second,
	})

	target := Target{
		OS:            "linux",
		Arch:          "amd64",
		OutputFormats: []string{"binary", "appimage", "deb", "zip"},
	}

	if err := builder.Available(context.Background(), target); err != nil {
		t.Fatalf("available: %v", err)
	}

	result, err := builder.Build(context.Background(), target)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(result.Artifacts) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(result.Artifacts))
	}

	for _, artifact := range result.Artifacts {
		path := filepath.Join(outputDir, filepath.FromSlash(artifact.Path))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
		if !strings.HasPrefix(artifact.Checksum, "sha256:") {
			t.Fatalf("expected checksum prefix, got %q", artifact.Checksum)
		}
		if _, err := os.Stat(path + ".sha256"); err != nil {
			t.Fatalf("expected checksum sidecar for %s: %v", path, err)
		}
	}
}

func writeTool(t *testing.T, path, content string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script based build test is not supported on windows")
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write tool %s: %v", path, err)
	}
}

func linuxToolScript() string {
	return `#!/bin/sh
set -eu
mkdir -p bin
case "$1" in
  task)
    printf 'linux-binary' > bin/MyApp
    chmod +x bin/MyApp
    ;;
  package)
    printf 'linux-binary' > bin/MyApp
    chmod +x bin/MyApp
    printf 'appimage' > bin/MyApp-amd64.AppImage
    printf 'deb' > bin/MyApp_amd64.deb
    ;;
  *)
    exit 1
    ;;
esac
`
}
