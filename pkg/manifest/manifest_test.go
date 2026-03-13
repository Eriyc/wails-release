package manifest

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPhase7ManifestContractSurface(t *testing.T) {
	manifest := &ReleaseManifest{
		SchemaVersion: 1,
		AppName:       "MyApp",
		Version:       "1.2.3",
		Channel:       "stable",
		Date:          time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		NativeCompat:  "2",
		Artifacts: []ArtifactEntry{
			{
				OS:       "linux",
				Arch:     "amd64",
				Format:   "appimage",
				URL:      "https://releases.example.com/download/1.2.3/linux/amd64/MyApp.AppImage",
				Checksum: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				Size:     123,
			},
		},
		Patches: []PatchEntry{
			{
				FromVersion: "1.2.2",
				OS:          "linux",
				Arch:        "amd64",
				FromHash:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ToHash:      "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				URL:         "https://releases.example.com/download/1.2.3/linux/amd64/MyApp.AppImage.patch",
				Checksum:    "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				Size:        45,
			},
		},
		FrontendBundles: []FrontendEntry{
			{
				Channel:  "beta",
				Version:  "1.2.3",
				CompatID: "2",
				URL:      "https://releases.example.com/download/frontend-beta-1.2.3.zip",
				Checksum: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
				Size:     67,
			},
		},
		Mandatory: &MandatoryInfo{
			MinVersion: "1.0.0",
			Message:    "Security update",
		},
	}

	if errs := Validate(manifest); len(errs) > 0 {
		t.Fatalf("expected manifest to validate, got %v", errs)
	}

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := WriteJSON(manifest, path); err != nil {
		t.Fatalf("write manifest json: %v", err)
	}
}

func TestGenerateExpandsURLTemplatesForArtifactsPatchesAndFrontendBundles(t *testing.T) {
	manifest, err := Generate(GenerateOpts{
		AppName:      "MyApp",
		Version:      "1.2.3",
		Channel:      "stable",
		NativeCompat: "2",
		ArtifactURLTemplate: "https://releases.example.com/download/{version}/{os}/{arch}/{name}",
		PatchURLTemplate:    "https://releases.example.com/download/{version}/{os}/{arch}/{name}",
		FrontendURLTemplate: "https://releases.example.com/download/{version}/{channel}/{name}",
		Artifacts: []GeneratedArtifact{
			{
				Name:     "MyApp.AppImage",
				OS:       "linux",
				Arch:     "amd64",
				Format:   "appimage",
				Checksum: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				Size:     123,
			},
		},
		Patches: []GeneratedPatch{
			{
				Name:        "MyApp.AppImage.patch",
				FromVersion: "1.2.2",
				OS:          "linux",
				Arch:        "amd64",
				FromHash:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ToHash:      "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Checksum:    "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				Size:        45,
			},
		},
		FrontendBundles: []GeneratedFrontendBundle{
			{
				Name:      "frontend-beta-1.2.3.zip",
				Channel:   "beta",
				Version:   "1.2.3",
				CompatID:  "2",
				Checksum:  "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
				Size:      67,
			},
		},
	})
	if err != nil {
		t.Fatalf("generate manifest: %v", err)
	}

	if got := manifest.Artifacts[0].URL; got != "https://releases.example.com/download/1.2.3/linux/amd64/MyApp.AppImage" {
		t.Fatalf("expected expanded artifact url, got %q", got)
	}
	if got := manifest.Patches[0].URL; got != "https://releases.example.com/download/1.2.3/linux/amd64/MyApp.AppImage.patch" {
		t.Fatalf("expected expanded patch url, got %q", got)
	}
	if got := manifest.FrontendBundles[0].URL; got != "https://releases.example.com/download/1.2.3/beta/frontend-beta-1.2.3.zip" {
		t.Fatalf("expected expanded frontend url, got %q", got)
	}
}
