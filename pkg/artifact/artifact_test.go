package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareOutputDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dist")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "old.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	if err := PrepareOutputDir(target, true); err != nil {
		t.Fatalf("prepare output dir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed, got %v", err)
	}
}

func TestPrepareUploadDirAndGitHubOutput(t *testing.T) {
	dir := t.TempDir()
	artifactFile := filepath.Join(dir, "artifact.txt")
	manifestFile := filepath.Join(dir, "manifest.json")
	outputDir := filepath.Join(dir, "upload")

	if err := os.WriteFile(artifactFile, []byte("artifact"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := os.WriteFile(manifestFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if err := PrepareUploadDir([]string{artifactFile}, manifestFile, outputDir); err != nil {
		t.Fatalf("prepare upload dir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "artifact.txt")); err != nil {
		t.Fatalf("expected copied artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "manifest.json")); err != nil {
		t.Fatalf("expected copied manifest: %v", err)
	}

	githubOutput := filepath.Join(dir, "github-output.txt")
	t.Setenv("GITHUB_OUTPUT", githubOutput)

	result := OutputResult{
		Version:      "1.2.3",
		ManifestPath: manifestFile,
		Artifacts:    []string{artifactFile},
	}
	if err := WriteGitHubOutput(result); err != nil {
		t.Fatalf("write github output: %v", err)
	}

	data, err := os.ReadFile(githubOutput)
	if err != nil {
		t.Fatalf("read github output: %v", err)
	}

	text := string(data)
	for _, expected := range []string{"version=1.2.3", "manifest_path=", "artifact_dir=", "artifact_count=1"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in github output, got %q", expected, text)
		}
	}
}
