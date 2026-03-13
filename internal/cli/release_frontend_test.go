package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eriyc/wailsrel/pkg/frontend"
)

func TestReleaseCommandDryRunDiscoversFrontendBundlesInUploads(t *testing.T) {
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

	buildDir := filepath.Join(repo, "frontend", "dist")
	if err := os.MkdirAll(filepath.Join(buildDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir frontend dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<!doctype html><html><body>release frontend</body></html>\n"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "assets", "app.js"), []byte("console.log('release frontend');\n"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}
	for _, channel := range []string{"stable", "beta"} {
		if _, err := frontend.BuildBundle(context.Background(), frontend.BundleOpts{
			WorkDir:       repo,
			OutputDir:     filepath.Join(repo, "dist"),
			OutputPath:    filepath.Join(repo, "dist", "frontend-"+channel+"-1.2.3.zip"),
			BuildCommand:  "go version",
			BuildDir:      "frontend/dist",
			CompatVer:     "2",
			CompatVersion: 2,
			Channel:       channel,
			Version:       "1.2.3",
		}); err != nil {
			t.Fatalf("build frontend bundle %s: %v", channel, err)
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

func TestDiscoverReleaseFrontendBundlesFailsForMalformedArchive(t *testing.T) {
	outputDir := t.TempDir()
	bundlePath := filepath.Join(outputDir, "frontend-stable-1.2.3.zip")
	if err := os.WriteFile(bundlePath, []byte("not a zip archive"), 0o644); err != nil {
		t.Fatalf("write malformed bundle archive: %v", err)
	}

	_, err := discoverReleaseFrontendBundles(outputDir)
	if err == nil {
		t.Fatal("expected malformed bundle archive to fail discovery")
	}
}

func TestDiscoverReleaseFrontendBundlesFailsWithoutBundleManifest(t *testing.T) {
	outputDir := t.TempDir()
	bundlePath := filepath.Join(outputDir, "frontend-stable-1.2.3.zip")
	writeTestBundleArchive(t, bundlePath, map[string]string{
		"index.html": "<!doctype html><html></html>\n",
	})

	_, err := discoverReleaseFrontendBundles(outputDir)
	if err == nil {
		t.Fatal("expected bundle archive without bundle.json to fail discovery")
	}
	if !strings.Contains(err.Error(), "bundle.json not found in archive") {
		t.Fatalf("expected missing bundle.json error, got %v", err)
	}
}

func TestDiscoverReleaseFrontendBundlesFailsForInvalidBundleManifestJSON(t *testing.T) {
	outputDir := t.TempDir()
	bundlePath := filepath.Join(outputDir, "frontend-stable-1.2.3.zip")
	writeTestBundleArchive(t, bundlePath, map[string]string{
		"bundle.json": "{invalid json",
	})

	_, err := discoverReleaseFrontendBundles(outputDir)
	if err == nil {
		t.Fatal("expected invalid bundle.json to fail discovery")
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("expected JSON decode error, got %v", err)
	}
}

func writeTestBundleArchive(t *testing.T, bundlePath string, files map[string]string) {
	t.Helper()

	file, err := os.Create(bundlePath)
	if err != nil {
		t.Fatalf("create bundle archive %s: %v", bundlePath, err)
	}

	writer := zip.NewWriter(file)
	for name, contents := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create bundle entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(contents)); err != nil {
			t.Fatalf("write bundle entry %s: %v", name, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close bundle archive writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close bundle archive file: %v", err)
	}
}
