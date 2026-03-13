package release

import (
	"strings"
	"testing"
	"time"
)

func TestManifestValidatesAgainstJSONSchema(t *testing.T) {
	manifest := &Manifest{
		SchemaVersion: 1,
		App: ManifestApp{
			Name:       "MyApp",
			Identifier: "com.example.myapp",
		},
		Release: ManifestRelease{
			Tag:      "v2.0.0",
			Version:  "2.0.0",
			Provider: "http",
		},
		GeneratedAt: time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		Artifacts: []ManifestArtifact{
			{
				Path:      "darwin/arm64/MyApp.app",
				AssetName: "MyApp-2.0.0-darwin-arm64-app.zip",
				OS:        "darwin",
				Arch:      "arm64",
				Format:    "app",
				Transport: "archive",
				Checksum:  "sha256:" + strings.Repeat("a", 64),
				Size:      123,
				URL:       "https://releases.example.com/download/MyApp-2.0.0-darwin-arm64-app.zip",
				Metadata: map[string]string{
					"channel":       "stable",
					"native_compat": "2",
				},
			},
		},
		Delta: &ManifestDelta{
			ManifestURL: "https://releases.example.com/delta/manifest.json",
		},
		Patches: []ManifestPatch{
			{
				FromVersion:  "1.9.0",
				Artifact:     "darwin/arm64/MyApp.app",
				URL:          "https://releases.example.com/download/MyApp-2.0.0-darwin-arm64-app.patch",
				Checksum:     "sha256:" + strings.Repeat("b", 64),
				FromChecksum: "sha256:" + strings.Repeat("c", 64),
				ToChecksum:   "sha256:" + strings.Repeat("d", 64),
				Size:         45,
			},
		},
		FrontendBundles: []ManifestFrontendBundle{
			{
				Channel:  "beta",
				Version:  "2.0.0",
				CompatID: "2",
				URL:      "https://releases.example.com/download/frontend-beta-2.0.0.zip",
				Checksum: "sha256:" + strings.Repeat("e", 64),
				Size:     67,
			},
		},
	}

	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("expected manifest to validate, got %v", err)
	}
}
