package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleCommandBuildsChannelBundleWithManifest(t *testing.T) {
	repo := t.TempDir()
	configPath := writeFrontendConfig(t, repo)

	tests := []struct {
		name    string
		channel string
	}{
		{name: "beta channel", channel: "beta"},
		{name: "stable channel", channel: "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCommand()
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stdout)
			cmd.SetArgs([]string{"--config", configPath, "bundle", "--channel=" + tt.channel})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute bundle command: %v\noutput:\n%s", err, stdout.String())
			}

			bundlePath := filepath.Join(repo, "dist", "frontend-"+tt.channel+"-2.0.0.zip")
			manifest := readBundleManifest(t, bundlePath)
			if got := strings.TrimSpace(asString(t, manifest["channel"])); got != tt.channel {
				t.Fatalf("expected bundle channel %q, got %q", tt.channel, got)
			}
			if got := strings.TrimSpace(asString(t, manifest["version"])); got != "2.0.0" {
				t.Fatalf("expected bundle version %q, got %q", "2.0.0", got)
			}
			if got := asInt(t, manifest["compat_version"]); got != 2 {
				t.Fatalf("expected compat version 2, got %d", got)
			}
		})
	}
}

func TestBundleCommandRejectsUnknownChannel(t *testing.T) {
	repo := t.TempDir()
	configPath := writeFrontendConfig(t, repo)

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "bundle", "--channel=nightly"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected bundle command to reject unknown channel")
	}
	if !strings.Contains(err.Error(), `unknown channel "nightly"`) {
		t.Fatalf("expected unknown channel error, got %v", err)
	}
}

func TestChannelCommandSwitchesActiveChannelMarker(t *testing.T) {
	repo := t.TempDir()
	configPath := writeFrontendConfig(t, repo)

	tests := []struct {
		name    string
		channel string
	}{
		{name: "switch to beta", channel: "beta"},
		{name: "switch back to stable", channel: "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCommand()
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stdout)
			cmd.SetArgs([]string{"--config", configPath, "channel", tt.channel})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute channel command: %v\noutput:\n%s", err, stdout.String())
			}

			markerPath := filepath.Join(repo, ".wailsrel", "frontend-channel")
			data, err := os.ReadFile(markerPath)
			if err != nil {
				t.Fatalf("read channel marker: %v", err)
			}
			if got := strings.TrimSpace(string(data)); got != tt.channel {
				t.Fatalf("expected active channel %q, got %q", tt.channel, got)
			}
		})
	}
}

func TestChannelCommandRejectsUnknownChannel(t *testing.T) {
	repo := t.TempDir()
	configPath := writeFrontendConfig(t, repo)

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "channel", "nightly"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected channel command to reject unknown channel")
	}
	if !strings.Contains(err.Error(), `unknown channel "nightly"`) {
		t.Fatalf("expected unknown channel error, got %v", err)
	}
}

func writeFrontendConfig(t *testing.T, repo string) string {
	t.Helper()

	configPath := filepath.Join(repo, "wailsrel.yaml")
	if err := os.WriteFile(configPath, []byte(`
app:
  name: "MyApp"
  identifier: "com.example.myapp"
version:
  source: file
  file: "VERSION"
targets:
  - os: linux
    arch: [amd64]
    output_formats: [binary]
    sign:
      provider: none
frontend:
  enabled: true
  compat_version: 2
  compat_auto_check: true
  bindings_dir: "frontend/bindings"
  build_dir: "frontend/dist"
  channels: [stable, beta]
  build_command: "go version"
output:
  dir: "dist"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "VERSION"), []byte("2.0.0\n"), 0o644); err != nil {
		t.Fatalf("write version file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "frontend", "bindings"), 0o755); err != nil {
		t.Fatalf("mkdir bindings dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "frontend", "bindings", "api.ts"), []byte("export const api = 1;\n"), 0o644); err != nil {
		t.Fatalf("write bindings file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "frontend", "dist", "assets"), 0o755); err != nil {
		t.Fatalf("mkdir frontend dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "frontend", "dist", "index.html"), []byte("<!doctype html><html><body>ok</body></html>\n"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "frontend", "dist", "assets", "app.js"), []byte("console.log('ok');\n"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}

	return configPath
}

func readBundleManifest(t *testing.T, bundlePath string) map[string]any {
	t.Helper()

	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("open bundle archive %s: %v", bundlePath, err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name != "bundle.json" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open bundle manifest: %v", err)
		}
		defer rc.Close()

		var manifest map[string]any
		if err := json.NewDecoder(rc).Decode(&manifest); err != nil {
			t.Fatalf("decode bundle manifest: %v", err)
		}
		return manifest
	}

	t.Fatalf("bundle archive %s did not contain bundle.json", bundlePath)
	return nil
}

func asString(t *testing.T, value any) string {
	t.Helper()

	str, ok := value.(string)
	if !ok {
		t.Fatalf("expected string value, got %T", value)
	}
	return str
}

func asInt(t *testing.T, value any) int {
	t.Helper()

	number, ok := value.(float64)
	if !ok {
		t.Fatalf("expected numeric value, got %T", value)
	}
	return int(number)
}
