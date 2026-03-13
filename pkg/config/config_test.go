package config

import "testing"

func TestValidateRequiresSchema2BuildHooksAndArtifacts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.App.Name = "MyApp"
	cfg.App.Identifier = "com.example.myapp"
	cfg.Version.Source = "file"
	cfg.Version.File = "VERSION"
	cfg.Targets = []TargetConfig{
		{
			ID:   "linux-amd64",
			OS:   "linux",
			Arch: "amd64",
			Build: BuildHookConfig{
				Argv: []string{"task", "build"},
			},
			Artifacts: []ArtifactSpec{
				{Format: "binary", Path: "bin/MyApp"},
			},
		},
	}

	if errs := cfg.Validate(); len(errs) != 0 {
		t.Fatalf("expected config to validate, got %+v", errs)
	}
}

func TestValidateRejectsArtifactWithoutPathOrGlob(t *testing.T) {
	cfg := DefaultConfig()
	cfg.App.Name = "MyApp"
	cfg.App.Identifier = "com.example.myapp"
	cfg.Version.Source = "file"
	cfg.Version.File = "VERSION"
	cfg.Targets = []TargetConfig{
		{
			ID:   "linux-amd64",
			OS:   "linux",
			Arch: "amd64",
			Build: BuildHookConfig{
				Argv: []string{"task", "build"},
			},
			Artifacts: []ArtifactSpec{
				{Format: "binary"},
			},
		},
	}

	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
}

func TestApplyDefaultsSetsManifestAndDeltaFlags(t *testing.T) {
	cfg := DefaultConfig()
	cfg.App.Name = "MyApp"
	cfg.App.Identifier = "com.example.myapp"
	cfg.Version.Source = "file"
	cfg.Version.File = "VERSION"
	cfg.Targets = []TargetConfig{
		{
			ID:   "windows-amd64",
			OS:   "windows",
			Arch: "amd64",
			Build: BuildHookConfig{
				Argv: []string{"task", "build"},
			},
			Artifacts: []ArtifactSpec{
				{Format: "exe", Path: "bin/MyApp.exe"},
				{Format: "msix", Path: "bin/MyApp.msix"},
			},
		},
	}
	cfg.applyDefaults()

	if cfg.Targets[0].Artifacts[0].IncludeInManifest == nil || !*cfg.Targets[0].Artifacts[0].IncludeInManifest {
		t.Fatal("expected exe artifact to be included in manifest by default")
	}
	if cfg.Targets[0].Artifacts[1].IncludeInManifest == nil || *cfg.Targets[0].Artifacts[1].IncludeInManifest {
		t.Fatal("expected msix artifact to be excluded from manifest by default")
	}
	if cfg.Targets[0].Artifacts[1].EnableDelta == nil || *cfg.Targets[0].Artifacts[1].EnableDelta {
		t.Fatal("expected msix artifact delta generation to be disabled by default")
	}
}
