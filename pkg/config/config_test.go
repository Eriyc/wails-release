package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAppliesEnvExpansionAndDefaults(t *testing.T) {
	t.Setenv("APP_AUTHOR", "Example Author")
	dir := t.TempDir()

	content := []byte(`
app:
  name: "MyApp"
  identifier: "com.example.myapp"
  author: "${APP_AUTHOR}"
targets:
  - os: darwin
    arch: [amd64]
    output_formats: [app]
`)

	if err := os.WriteFile(filepath.Join(dir, DefaultFileName), content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.App.Author != "Example Author" {
		t.Fatalf("expected env expansion, got %q", cfg.App.Author)
	}
	if cfg.Version.Source != "git" {
		t.Fatalf("expected default version source git, got %q", cfg.Version.Source)
	}
	if cfg.Output.Dir != "dist" {
		t.Fatalf("expected default output dir dist, got %q", cfg.Output.Dir)
	}
	if !cfg.CI.Artifacts.Upload {
		t.Fatal("expected CI artifact upload default to true")
	}
}

func TestValidateReportsFatalErrors(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Identifier: "",
		},
		Version: VersionConfig{
			Source: "bad",
		},
		Targets: []TargetConfig{
			{
				OS:            "solaris",
				Arch:          []string{"mips"},
				OutputFormats: []string{"unknown"},
				Sign: SignConfig{
					Provider: "mystery",
				},
			},
		},
		Output: OutputConfig{
			Dir: "",
		},
		CI: CIConfig{
			Provider: "circle",
			Timeout:  "later",
		},
	}

	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}

	var fatalCount int
	for _, err := range errs {
		if err.Fatal {
			fatalCount++
		}
	}

	if fatalCount == 0 {
		t.Fatal("expected at least one fatal error")
	}
}

func TestValidateRejectsUnsupportedSigningConfiguration(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Name:       "MyApp",
			Identifier: "com.example.myapp",
		},
		Version: VersionConfig{
			Source: "git",
		},
		Targets: []TargetConfig{
			{
				OS:            "linux",
				Arch:          []string{"amd64"},
				OutputFormats: []string{"binary"},
				Sign: SignConfig{
					Provider: "azure",
				},
			},
		},
		Output: OutputConfig{
			Dir:          "dist",
			ManifestFile: "manifest.json",
		},
		CI: CIConfig{
			Provider: "github",
			Timeout:  "30m",
		},
	}

	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}

	var found bool
	for _, err := range errs {
		if err.Field == "targets[0].sign.provider" && strings.Contains(err.Message, "not supported") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unsupported signing provider error, got %+v", errs)
	}
}
