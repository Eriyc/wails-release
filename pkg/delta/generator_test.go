package delta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabstv/go-bsdiff/pkg/bspatch"
)

func TestPlanAndGeneratePatchFromCachedArtifact(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "dist")
	cacheDir := filepath.Join(root, ".wailsrel", "cache")

	currentPath := filepath.Join(outputDir, "linux", "amd64", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(currentPath, []byte("new release binary"), 0o644); err != nil {
		t.Fatalf("write current: %v", err)
	}
	if err := os.WriteFile(currentPath+".sha256", []byte("ignore"), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	previousPath := filepath.Join(cacheDir, "v1.2.3", "linux", "amd64", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(previousPath), 0o755); err != nil {
		t.Fatalf("mkdir previous: %v", err)
	}
	if err := os.WriteFile(previousPath, []byte("old release binary"), 0o644); err != nil {
		t.Fatalf("write previous: %v", err)
	}

	generator := NewGenerator(Options{
		OutputDir:    outputDir,
		CacheDir:     cacheDir,
		FromVersions: 3,
		Source:       "local-cache",
		TagPrefix:    "v",
	})

	plan, err := generator.Plan()
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Pending) != 1 {
		t.Fatalf("expected 1 pending patch, got %d", len(plan.Pending))
	}
	if got := plan.Pending[0].Artifact; got != "linux/amd64/MyApp.AppImage" {
		t.Fatalf("unexpected artifact %q", got)
	}

	result, err := generator.Generate(plan)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(result.Generated) != 1 {
		t.Fatalf("expected 1 generated patch, got %d", len(result.Generated))
	}
	if !strings.HasSuffix(result.Generated[0].Patch, "MyApp.AppImage.bsdiff") {
		t.Fatalf("unexpected patch path %q", result.Generated[0].Patch)
	}

	oldBytes, err := os.ReadFile(previousPath)
	if err != nil {
		t.Fatalf("read previous: %v", err)
	}
	patchBytes, err := os.ReadFile(result.Generated[0].Patch)
	if err != nil {
		t.Fatalf("read patch: %v", err)
	}
	reconstructed, err := bspatch.Bytes(oldBytes, patchBytes)
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	currentBytes, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read current: %v", err)
	}
	if string(reconstructed) != string(currentBytes) {
		t.Fatalf("patch did not reconstruct current artifact")
	}
}

func TestPlanSkipsDirectoriesAndUnchangedArtifacts(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "dist")
	cacheDir := filepath.Join(root, ".wailsrel", "cache")

	appDir := filepath.Join(outputDir, "darwin", "arm64", "MyApp.app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir app dir: %v", err)
	}

	currentBinary := filepath.Join(outputDir, "linux", "amd64", "MyApp")
	if err := os.MkdirAll(filepath.Dir(currentBinary), 0o755); err != nil {
		t.Fatalf("mkdir binary dir: %v", err)
	}
	if err := os.WriteFile(currentBinary, []byte("same bytes"), 0o644); err != nil {
		t.Fatalf("write current binary: %v", err)
	}

	previousBinary := filepath.Join(cacheDir, "v1.0.0", "linux", "amd64", "MyApp")
	if err := os.MkdirAll(filepath.Dir(previousBinary), 0o755); err != nil {
		t.Fatalf("mkdir previous binary dir: %v", err)
	}
	if err := os.WriteFile(previousBinary, []byte("same bytes"), 0o644); err != nil {
		t.Fatalf("write previous binary: %v", err)
	}

	generator := NewGenerator(Options{
		OutputDir:    outputDir,
		CacheDir:     cacheDir,
		FromVersions: 1,
		Source:       "local-cache",
		TagPrefix:    "v",
	})

	plan, err := generator.Plan()
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Pending) != 0 {
		t.Fatalf("expected no pending patches, got %d", len(plan.Pending))
	}

	var sawDirSkip bool
	var sawUnchangedSkip bool
	for _, skipped := range plan.Skipped {
		if skipped.Artifact == "darwin/arm64/MyApp.app" && strings.Contains(skipped.Reason, "directory artifacts") {
			sawDirSkip = true
		}
		if skipped.Artifact == "linux/amd64/MyApp" && strings.Contains(skipped.Reason, "unchanged") {
			sawUnchangedSkip = true
		}
	}

	if !sawDirSkip {
		t.Fatal("expected directory skip")
	}
	if !sawUnchangedSkip {
		t.Fatal("expected unchanged artifact skip")
	}
}

func TestDiscoverCachedVersionsSortsAndLimits(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"v1.0.0", "v1.2.0", "v1.1.5", "notes"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	versions, err := discoverCachedVersions(root, "v", 2)
	if err != nil {
		t.Fatalf("discover versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].name != "v1.2.0" || versions[1].name != "v1.1.5" {
		t.Fatalf("unexpected versions order: %#v", versions)
	}
}
