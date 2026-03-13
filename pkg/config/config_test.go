package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
	if cfg.Release.Provider != "github" {
		t.Fatalf("expected default release provider github, got %q", cfg.Release.Provider)
	}
	if cfg.Release.HTTP.DownloadPathPrefix != "/download" {
		t.Fatalf("expected default download path prefix /download, got %q", cfg.Release.HTTP.DownloadPathPrefix)
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

func TestValidateRequiresHTTPReleaseBaseURL(t *testing.T) {
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
					Provider: "none",
				},
			},
		},
		Release: ReleaseConfig{
			Provider: "http",
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
	var found bool
	for _, err := range errs {
		if err.Field == "release.http.base_url" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected release.http.base_url validation error, got %+v", errs)
	}
}

func TestValidateRequiresURLManifestSource(t *testing.T) {
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
					Provider: "none",
				},
			},
		},
		Delta: DeltaConfig{
			Enabled:      true,
			Algorithm:    "bsdiff",
			FromVersions: 1,
			OldArtifacts: OldArtifactsConfig{
				Source: "url",
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
	var found bool
	for _, err := range errs {
		if err.Field == "delta.old_artifacts.manifest_url" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected delta.old_artifacts.manifest_url validation error, got %+v", errs)
	}
}

func TestConfigMarshalUnmarshalRoundTrip(t *testing.T) {
	original := Config{
		App: AppConfig{
			Name:        "MyApp",
			Identifier:  "com.example.myapp",
			Description: "Updater fixture",
			Author:      "Example",
			URL:         "https://example.com",
		},
		Version: VersionConfig{
			Source:           "git",
			File:             "VERSION",
			TagPrefix:        "v",
			PrereleaseFormat: "beta.{n}",
		},
		Targets: []TargetConfig{
			{
				OS:            "darwin",
				Arch:          []string{"arm64", "amd64"},
				OutputFormats: []string{"app", "dmg"},
				Sign: SignConfig{
					Provider: "apple",
					Identity: "Developer ID Application: Example (TEAMID)",
					Notarize: true,
					AppleID:  "dev@example.com",
					Password: "app-password",
					TeamID:   "TEAMID",
				},
			},
		},
		Installers: InstallerConfig{
			NSIS: NSISInstallerConfig{
				License:      "LICENSE.txt",
				Icon:         "icon.ico",
				CustomScript: "installer.nsi",
			},
			DMG: DMGInstallerConfig{
				Background: "background.png",
				IconSize:   96,
				WindowSize: []int{960, 540},
			},
			Deb: DebInstallerConfig{
				Depends:  []string{"libwebkit2gtk-4.1-0"},
				Section:  "utils",
				Priority: "optional",
			},
		},
		Delta: DeltaConfig{
			Enabled:      true,
			Algorithm:    "bsdiff",
			FromVersions: 5,
			OldArtifacts: OldArtifactsConfig{
				Source:       "url",
				Repository:   "acme/myapp",
				CacheDir:     ".wailsrel/cache",
				ManifestURL:  "https://releases.example.com/manifest.json",
				AuthTokenEnv: "WAILSREL_TOKEN",
			},
		},
		Frontend: FrontendConfig{
			Enabled:         true,
			CompatVersion:   3,
			CompatAutoCheck: true,
			BindingsDir:     "frontend/bindings",
			BuildDir:        "frontend/dist",
			Channels:        []string{"stable", "beta"},
			BuildCommand:    "npm run build",
		},
		Update: UpdateConfig{
			ManifestURL:    "https://releases.example.com/manifest.json",
			CheckInterval:  "1h",
			Channels:       []string{"stable", "beta"},
			AllowDowngrade: true,
			MandatoryMin:   "1.5.0",
		},
		Release: ReleaseConfig{
			Provider: "http",
			GitHub: ReleaseGitHubConfig{
				Repository: "acme/myapp",
				APIBaseURL: "https://api.github.example.com",
			},
			HTTP: ReleaseHTTPConfig{
				BaseURL:            "https://releases.example.com",
				ManifestPath:       "/manifest.json",
				DeltaManifestPath:  "/delta/manifest.json",
				DownloadPathPrefix: "/download",
			},
		},
		Output: OutputConfig{
			Dir:          "dist",
			ManifestFile: "manifest.json",
			Clean:        true,
		},
		CI: CIConfig{
			Provider: "github",
			Timeout:  "45m",
			Artifacts: CIArtifactsConfig{
				Upload:        true,
				RetentionDays: 30,
			},
		},
	}

	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	var decoded Config
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("expected config round-trip to preserve values:\noriginal=%#v\ndecoded=%#v", original, decoded)
	}
}
