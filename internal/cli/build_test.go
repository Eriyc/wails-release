package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eriyc/wailsrel/pkg/frontend"
)

func TestBuildDryRunWarnsOnCompatMismatch(t *testing.T) {
	repo := t.TempDir()
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
  compat_version: 1
  compat_auto_check: true
  bindings_dir: "frontend/bindings"
  build_dir: "frontend/dist"
  channels: [stable]
  build_command: "npm run build"
output:
  dir: "dist"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "VERSION"), []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatalf("write version file: %v", err)
	}

	bindingsDir := filepath.Join(repo, "frontend", "bindings")
	if err := os.MkdirAll(bindingsDir, 0o755); err != nil {
		t.Fatalf("mkdir bindings dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bindingsDir, "backend.ts"), []byte("export const api = 2;\n"), 0o644); err != nil {
		t.Fatalf("write bindings: %v", err)
	}
	if err := frontend.WriteCompatSnapshot(frontendCompatSnapshotPath(repo), frontend.CompatSnapshot{
		CompatVersion: 1,
		BindingsHash:  "old-hash",
	}); err != nil {
		t.Fatalf("write compat snapshot: %v", err)
	}

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "--dry-run", "--json", "build"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute build dry run: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, `"warnings": [`) {
		t.Fatalf("expected warnings array in output, got %q", output)
	}
	if !strings.Contains(output, `frontend bindings changed but frontend.compat_version is still 1`) {
		t.Fatalf("expected compat warning in output, got %q", output)
	}
}
