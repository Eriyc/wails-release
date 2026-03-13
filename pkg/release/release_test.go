package release

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/you/wailsrel/pkg/build"
	"github.com/you/wailsrel/pkg/config"
	"github.com/you/wailsrel/pkg/delta"
)

func TestResolvers(t *testing.T) {
	github := NewGitHubResolver("acme/myapp", "https://api.github.com")
	if got := github.ArtifactURL("v1.2.3", "manifest.json"); got != "https://github.com/acme/myapp/releases/download/v1.2.3/manifest.json" {
		t.Fatalf("unexpected github artifact url %q", got)
	}

	http := NewHTTPResolver("https://releases.example.com/", "/manifest.json", "/delta/manifest.json", "/download")
	if got := http.ArtifactURL("v1.2.3", "artifact.zip"); got != "https://releases.example.com/download/v1.2.3/artifact.zip" {
		t.Fatalf("unexpected http artifact url %q", got)
	}
	if got := http.ManifestURL("ignored"); got != "https://releases.example.com/manifest.json" {
		t.Fatalf("unexpected manifest url %q", got)
	}
}

func TestPrepareBundleCanonicalNamesAndManifest(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "dist")

	fileArtifactPath := filepath.Join(outputDir, "linux", "amd64", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(fileArtifactPath), 0o755); err != nil {
		t.Fatalf("mkdir file artifact: %v", err)
	}
	if err := os.WriteFile(fileArtifactPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("write file artifact: %v", err)
	}

	dirArtifactPath := filepath.Join(outputDir, "darwin", "arm64", "MyApp.app", "Contents", "MacOS", "MyApp")
	if err := os.MkdirAll(filepath.Dir(dirArtifactPath), 0o755); err != nil {
		t.Fatalf("mkdir dir artifact: %v", err)
	}
	if err := os.WriteFile(dirArtifactPath, []byte("app-binary"), 0o755); err != nil {
		t.Fatalf("write dir artifact: %v", err)
	}

	patchPath := filepath.Join(outputDir, "delta", "v1.0.0", "linux", "amd64", "MyApp.AppImage.patch")
	if err := os.MkdirAll(filepath.Dir(patchPath), 0o755); err != nil {
		t.Fatalf("mkdir patch: %v", err)
	}
	if err := os.WriteFile(patchPath, []byte("patch"), 0o644); err != nil {
		t.Fatalf("write patch: %v", err)
	}

	nowUTC = func() time.Time {
		return time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	}
	defer func() {
		nowUTC = func() time.Time { return time.Now().UTC() }
	}()

	bundle, err := PrepareBundle(BundleOptions{
		App: config.AppConfig{
			Name:       "MyApp",
			Identifier: "com.example.myapp",
		},
		OutputDir: outputDir,
		TempDir:   filepath.Join(root, ".release"),
		Tag:       "v1.2.3",
		Version:   "1.2.3",
		Resolver:  NewHTTPResolver("https://releases.example.com", "/manifest.json", "/delta/manifest.json", "/download"),
		Artifacts: []build.Artifact{
			{
				Path:     "linux/amd64/MyApp.AppImage",
				OS:       "linux",
				Arch:     "amd64",
				Format:   "appimage",
				Checksum: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				Size:     6,
			},
			{
				Path:     "darwin/arm64/MyApp.app",
				OS:       "darwin",
				Arch:     "arm64",
				Format:   "app",
				Checksum: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
				Size:     10,
			},
		},
		Delta: &delta.Result{
			ManifestPath: filepath.Join(outputDir, "delta", "manifest.json"),
			Generated: []delta.Generated{
				{
					FromVersion:  "v1.0.0",
					Artifact:     "linux/amd64/MyApp.AppImage",
					ArtifactKind: delta.ArtifactFile,
					Patch:        patchPath,
					Checksum:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					FromChecksum: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					ToChecksum:   "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
					FromSize:     3,
					ToSize:       6,
					Size:         5,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}

	if len(bundle.Artifacts) != 2 {
		t.Fatalf("expected 2 published artifacts, got %d", len(bundle.Artifacts))
	}
	names := []string{bundle.Artifacts[0].AssetName, bundle.Artifacts[1].AssetName}
	slices.Sort(names)
	expectedNames := []string{"MyApp-1.2.3-darwin-arm64-app.zip", "MyApp-1.2.3-linux-amd64-appimage.AppImage"}
	if strings.Join(names, ",") != strings.Join(expectedNames, ",") {
		t.Fatalf("unexpected asset names %v", names)
	}
	if bundle.Manifest.Release.Provider != ProviderHTTP {
		t.Fatalf("expected provider %s, got %s", ProviderHTTP, bundle.Manifest.Release.Provider)
	}
	if bundle.Manifest.Delta == nil || bundle.Manifest.Delta.ManifestURL != "https://releases.example.com/delta/manifest.json" {
		t.Fatalf("unexpected delta manifest %+v", bundle.Manifest.Delta)
	}

	var archiveEntry *ManifestArtifact
	for i := range bundle.Manifest.Artifacts {
		if bundle.Manifest.Artifacts[i].Path == "darwin/arm64/MyApp.app" {
			archiveEntry = &bundle.Manifest.Artifacts[i]
			break
		}
	}
	if archiveEntry == nil {
		t.Fatal("expected darwin archive entry in manifest")
	}
	if archiveEntry.Transport != "archive" {
		t.Fatalf("expected archive transport, got %q", archiveEntry.Transport)
	}

	var archiveSource string
	for _, artifact := range bundle.Artifacts {
		if artifact.LogicalPath == "darwin/arm64/MyApp.app" {
			archiveSource = artifact.SourcePath
			break
		}
	}
	if archiveSource == "" {
		t.Fatal("expected published archive source path")
	}
	checksum, err := build.ComputeChecksum(archiveSource)
	if err != nil {
		t.Fatalf("compute archive checksum: %v", err)
	}
	info, err := os.Stat(archiveSource)
	if err != nil {
		t.Fatalf("stat archive source: %v", err)
	}
	if archiveEntry.Checksum != "sha256:"+checksum {
		t.Fatalf("expected archive checksum %q, got %q", "sha256:"+checksum, archiveEntry.Checksum)
	}
	if archiveEntry.Size != info.Size() {
		t.Fatalf("expected archive size %d, got %d", info.Size(), archiveEntry.Size)
	}
	if err := ValidateManifest(bundle.Manifest); err != nil {
		t.Fatalf("validate manifest: %v", err)
	}

	var sawManifest, sawDeltaManifest, sawDeltaPatch bool
	for _, upload := range bundle.Uploads {
		switch upload.Name {
		case ManifestAssetName:
			sawManifest = true
		case DeltaManifestAssetName:
			sawDeltaManifest = true
		}
		if strings.Contains(upload.Name, "delta-from-v1.0.0") {
			sawDeltaPatch = true
		}
	}
	if !sawManifest || !sawDeltaManifest || !sawDeltaPatch {
		t.Fatalf("missing expected uploads: manifest=%t deltaManifest=%t deltaPatch=%t", sawManifest, sawDeltaManifest, sawDeltaPatch)
	}
	if _, err := os.Stat(bundle.DeltaManifestPath); err != nil {
		t.Fatalf("expected delta manifest file %s: %v", bundle.DeltaManifestPath, err)
	}
}
