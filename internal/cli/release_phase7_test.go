package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCommandDryRunIncludesFrontendBundlesInUploads(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, "wailsrel.yaml")
	if err := os.WriteFile(configPath, []byte(`
app:
  name: "MyApp"
  identifier: "com.example.myapp"
version:
  source: file
  file: "VERSION"
  tag_prefix: "v"
targets:
  - os: linux
    arch: [amd64]
    output_formats: [appimage]
delta:
  enabled: true
  old_artifacts:
    source: local-cache
frontend:
  enabled: true
  compat_version: 2
  compat_auto_check: true
  bindings_dir: "frontend/bindings"
  build_dir: "frontend/dist"
  channels: [stable, beta]
  build_command: "go version"
release:
  provider: http
  github:
    repository: "acme/myapp"
  http:
    base_url: "https://releases.example.com"
    manifest_path: "/manifest.json"
    delta_manifest_path: "/delta/manifest.json"
    download_path_prefix: "/download"
output:
  dir: "dist"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "VERSION"), []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatalf("write version file: %v", err)
	}

	artifactPath := filepath.Join(repo, "dist", "linux", "amd64", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("mkdir artifact: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	deltaManifest := filepath.Join(repo, "dist", "delta", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(deltaManifest), 0o755); err != nil {
		t.Fatalf("mkdir delta manifest dir: %v", err)
	}
	if err := os.WriteFile(deltaManifest, []byte(`{
  "schema_version": 1,
  "algorithm": "bsdiff",
  "generated_at": "2026-03-13T12:00:00Z",
  "patches": []
}`), 0o644); err != nil {
		t.Fatalf("write delta manifest: %v", err)
	}

	for _, name := range []string{
		"frontend-stable-1.2.3.zip",
		"frontend-beta-1.2.3.zip",
	} {
		if err := os.WriteFile(filepath.Join(repo, "dist", name), []byte("bundle"), 0o644); err != nil {
			t.Fatalf("write frontend bundle %s: %v", name, err)
		}
	}

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "--dry-run", "--json", "release"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute release dry run: %v", err)
	}

	output := stdout.String()
	for _, expected := range []string{
		`frontend-stable-1.2.3.zip`,
		`frontend-beta-1.2.3.zip`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in release output, got %q", expected, output)
		}
	}
}
