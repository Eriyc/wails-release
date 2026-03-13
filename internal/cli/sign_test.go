package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eriyc/wailsrel/pkg/config"
)

func TestSignCommandSkipsWhenProviderIsNone(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, "wailsrel.yaml")
	if err := os.WriteFile(configPath, []byte(`
app:
  name: "Test App"
  identifier: "com.example.test"
targets:
  - os: linux
    arch: [amd64]
    output_formats: [appimage]
    sign:
      provider: none
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	artifactPath := filepath.Join(repo, "dist", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "sign", artifactPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute sign: %v", err)
	}

	if !strings.Contains(stdout.String(), "Skipped signing") {
		t.Fatalf("expected skip output, got %q", stdout.String())
	}
}

func TestResolveSignConfigRejectsAmbiguousTarget(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{OS: "windows", Sign: config.SignConfig{Provider: "azure", Profile: "one"}},
			{OS: "windows", Sign: config.SignConfig{Provider: "azure", Profile: "two"}},
		},
	}

	_, _, err := resolveSignConfig(cfg, "MyApp.exe", "", "")
	if err == nil || !strings.Contains(err.Error(), "multiple signing configurations") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}
