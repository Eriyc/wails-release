package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Eriyc/wailsrel/pkg/config"
)

func TestExpandMatrixFlattensTargets(t *testing.T) {
	matrix := ExpandMatrix([]config.TargetConfig{
		{
			ID:   "linux-amd64",
			OS:   "linux",
			Arch: "amd64",
			Build: config.BuildHookConfig{
				Argv: []string{"task", "build"},
			},
			Artifacts: []config.ArtifactSpec{{Format: "binary", Path: "bin/MyApp"}},
		},
	})

	if len(matrix) != 1 {
		t.Fatalf("expected 1 target, got %d", len(matrix))
	}
	if matrix[0].ID != "linux-amd64" {
		t.Fatalf("unexpected target %+v", matrix[0])
	}
}

func TestBuildRunsHookAndStagesArtifacts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs are not supported on windows")
	}

	projectDir := t.TempDir()
	outputDir := filepath.Join(projectDir, "dist")
	toolsDir := filepath.Join(projectDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	writeTool(t, filepath.Join(toolsDir, "task"), `#!/bin/sh
set -eu
mkdir -p bin
printf 'binary' > bin/MyApp
printf 'msix' > bin/MyApp-amd64.msix
`)
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	builder := NewBuilder(Options{
		ProjectDir: projectDir,
		OutputDir:  outputDir,
		AppName:    "MyApp",
		Version:    "1.2.3",
		Tag:        "v1.2.3",
		Timeout:    5 * time.Second,
	})

	target := Target{
		ID:   "windows-amd64",
		OS:   "windows",
		Arch: "amd64",
		Build: config.BuildHookConfig{
			Argv:     []string{"task", "build", "ARCH={{arch}}"},
			Requires: []string{"task"},
		},
		Artifacts: []config.ArtifactSpec{
			{Format: "exe", Path: "bin/MyApp", IncludeInManifest: boolPtr(true), EnableDelta: boolPtr(true)},
			{Format: "msix", Glob: "bin/*.msix", IncludeInManifest: boolPtr(false), EnableDelta: boolPtr(false)},
		},
	}

	if err := builder.Available(context.Background(), target); err != nil {
		t.Fatalf("available: %v", err)
	}
	result, err := builder.Build(context.Background(), target)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(result.Artifacts))
	}
	if !result.Artifacts[0].EnableDelta {
		t.Fatal("expected first artifact delta flag to be true")
	}
	if result.Artifacts[1].IncludeInManifest {
		t.Fatal("expected msix artifact to be excluded from manifest")
	}
	for _, artifact := range result.Artifacts {
		path := filepath.Join(outputDir, filepath.FromSlash(artifact.Path))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing staged artifact %s: %v", path, err)
		}
		if !strings.HasPrefix(artifact.Checksum, "sha256:") {
			t.Fatalf("unexpected checksum %q", artifact.Checksum)
		}
	}
}

func TestBuildGlobFailure(t *testing.T) {
	builder := NewBuilder(Options{ProjectDir: t.TempDir(), OutputDir: t.TempDir(), AppName: "MyApp"})
	target := Target{
		ID:   "linux-amd64",
		OS:   "linux",
		Arch: "amd64",
		Build: config.BuildHookConfig{
			Argv: []string{"true"},
		},
		Artifacts: []config.ArtifactSpec{
			{Format: "binary", Glob: "bin/*.missing"},
		},
	}

	_, err := builder.stageArtifacts(target, builder.templateVars(target))
	if err == nil || !strings.Contains(err.Error(), "glob matched no files") {
		t.Fatalf("expected glob mismatch error, got %v", err)
	}
}

func writeTool(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write tool %s: %v", path, err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
