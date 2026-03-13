package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeltaCommandDryRunJSON(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, "wailsrel.yaml")
	if err := os.WriteFile(configPath, []byte(`
app:
  name: "Test App"
  identifier: "com.example.test"
version:
  tag_prefix: "v"
targets:
  - os: linux
    arch: [amd64]
    output_formats: [appimage]
delta:
  enabled: true
  from_versions: 1
  old_artifacts:
    source: local-cache
    cache_dir: ".wailsrel/cache"
output:
  dir: "dist"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	currentPath := filepath.Join(repo, "dist", "linux", "amd64", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(currentPath, []byte("new binary"), 0o644); err != nil {
		t.Fatalf("write current: %v", err)
	}

	previousPath := filepath.Join(repo, ".wailsrel", "cache", "v1.0.0", "linux", "amd64", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(previousPath), 0o755); err != nil {
		t.Fatalf("mkdir previous: %v", err)
	}
	if err := os.WriteFile(previousPath, []byte("old binary"), 0o644); err != nil {
		t.Fatalf("write previous: %v", err)
	}

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "--dry-run", "--json", "delta"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute delta dry run: %v", err)
	}

	output := stdout.String()
	for _, expected := range []string{`"from_version": "v1.0.0"`, `"artifact": "linux/amd64/MyApp.AppImage"`, `"pending"`, `.patch"`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in output, got %q", expected, output)
		}
	}
}

func TestDeltaCommandGeneratesPatch(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, "wailsrel.yaml")
	if err := os.WriteFile(configPath, []byte(`
app:
  name: "Test App"
  identifier: "com.example.test"
version:
  tag_prefix: "v"
targets:
  - os: linux
    arch: [amd64]
    output_formats: [appimage]
delta:
  enabled: true
  from_versions: 1
  old_artifacts:
    source: local-cache
    cache_dir: ".wailsrel/cache"
output:
  dir: "dist"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	currentPath := filepath.Join(repo, "dist", "linux", "amd64", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(currentPath, []byte("new binary"), 0o644); err != nil {
		t.Fatalf("write current: %v", err)
	}

	previousPath := filepath.Join(repo, ".wailsrel", "cache", "v1.0.0", "linux", "amd64", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(previousPath), 0o755); err != nil {
		t.Fatalf("mkdir previous: %v", err)
	}
	if err := os.WriteFile(previousPath, []byte("old binary"), 0o644); err != nil {
		t.Fatalf("write previous: %v", err)
	}

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "delta"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute delta: %v", err)
	}

	patchPath := filepath.Join(repo, "dist", "delta", "v1.0.0", "linux", "amd64", "MyApp.AppImage.patch")
	if _, err := os.Stat(patchPath); err != nil {
		t.Fatalf("expected patch %s: %v", patchPath, err)
	}
	manifestPath := filepath.Join(repo, "dist", "delta", "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest %s: %v", manifestPath, err)
	}
	if !strings.Contains(stdout.String(), "Generated 1 patch(es)") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "smaller") {
		t.Fatalf("expected savings output, got %q", stdout.String())
	}
}
