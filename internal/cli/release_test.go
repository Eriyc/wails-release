package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCommandDryRunJSON(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, "wailsrel.yaml")
	if err := os.WriteFile(configPath, []byte(`
schema: 2
app:
  name: "MyApp"
  identifier: "com.example.myapp"
version:
  source: file
  file: "VERSION"
  tag_prefix: "v"
targets:
  - id: linux-amd64
    os: linux
    arch: amd64
    build:
      argv: ["task", "build"]
    artifacts:
      - format: appimage
        path: "bin/MyApp.AppImage"
delta:
  enabled: true
  old_artifacts:
    source: local-cache
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
  "patches": [
    {
      "from_version": "v1.0.0",
      "artifact": "linux/amd64/MyApp.AppImage",
      "patch": "delta/v1.0.0/linux/amd64/MyApp.AppImage.patch",
      "artifact_kind": "file",
      "from_sha256": "1111111111111111111111111111111111111111111111111111111111111111",
      "to_sha256": "2222222222222222222222222222222222222222222222222222222222222222",
      "patch_sha256": "3333333333333333333333333333333333333333333333333333333333333333",
      "from_size": 4,
      "to_size": 6,
      "patch_size": 3
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write delta manifest: %v", err)
	}
	patchPath := filepath.Join(repo, "dist", "delta", "v1.0.0", "linux", "amd64", "MyApp.AppImage.patch")
	if err := os.MkdirAll(filepath.Dir(patchPath), 0o755); err != nil {
		t.Fatalf("mkdir patch dir: %v", err)
	}
	if err := os.WriteFile(patchPath, []byte("ptc"), 0o644); err != nil {
		t.Fatalf("write patch: %v", err)
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
		`"provider": "http"`,
		`"tag": "v1.2.3"`,
		`"manifest_url": "https://releases.example.com/manifest.json"`,
		`"delta_manifest_url": "https://releases.example.com/delta/manifest.json"`,
		`"MyApp-1.2.3-linux-amd64-appimage.AppImage"`,
		`"delta-manifest.json"`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in output, got %q", expected, output)
		}
	}
}
