package delta

import (
	"archive/zip"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAndApplyFilePatchRoundTrip(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.bin")
	newPath := filepath.Join(root, "new.bin")
	patchPath := filepath.Join(root, "artifact.patch")
	outputPath := filepath.Join(root, "applied.bin")

	if err := os.WriteFile(oldPath, []byte("old release binary"), 0o755); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new release binary with more bytes"), 0o755); err != nil {
		t.Fatalf("write new: %v", err)
	}

	info, err := Generate(oldPath, newPath, patchPath)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if info.ArtifactKind != ArtifactFile {
		t.Fatalf("unexpected artifact kind %s", info.ArtifactKind)
	}

	applied, err := Apply(oldPath, patchPath, outputPath)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Checksum != info.ToChecksum {
		t.Fatalf("checksum mismatch: %s != %s", applied.Checksum, info.ToChecksum)
	}
}

func TestGenerateRepresentativePatchSizeRatio(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.bin")
	newPath := filepath.Join(root, "new.bin")
	patchPath := filepath.Join(root, "artifact.patch")

	oldBytes := deterministicBytes(1, 500000)
	newBytes := append([]byte(nil), oldBytes...)
	copy(newBytes[100000:200000], deterministicBytes(2, 100000))

	if err := os.WriteFile(oldPath, oldBytes, 0o644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := os.WriteFile(newPath, newBytes, 0o644); err != nil {
		t.Fatalf("write new: %v", err)
	}

	info, err := Generate(oldPath, newPath, patchPath)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	ratio := float64(info.PatchSize) / float64(info.ToSize)
	if ratio < 0.10 || ratio > 0.30 {
		t.Fatalf("expected representative patch ratio between 10%% and 30%%, got %.3f", ratio)
	}
	if info.SavingsPercent < 70 || info.SavingsPercent > 90 {
		t.Fatalf("expected representative savings between 70%% and 90%%, got %.1f%%", info.SavingsPercent)
	}
}

func TestGenerateAndApplyDirectoryPatchRoundTrip(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old", "MyApp.app")
	newPath := filepath.Join(root, "new", "MyApp.app")
	patchPath := filepath.Join(root, "MyApp.app.patch")
	outputPath := filepath.Join(root, "applied", "MyApp.app")

	writeAppBundle(t, oldPath, "old")
	writeAppBundle(t, newPath, "new")

	info, err := Generate(oldPath, newPath, patchPath)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if info.ArtifactKind != ArtifactDirectory {
		t.Fatalf("unexpected artifact kind %s", info.ArtifactKind)
	}

	applied, err := Apply(oldPath, patchPath, outputPath)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Checksum != info.ToChecksum {
		t.Fatalf("checksum mismatch: %s != %s", applied.Checksum, info.ToChecksum)
	}
}

func TestGeneratorFetchesGitHubAssetsAndWritesManifest(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "dist")
	cacheDir := filepath.Join(root, ".wailsrel", "cache")

	currentPath := filepath.Join(outputDir, "linux", "amd64", "MyApp.AppImage")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(currentPath, []byte("new release binary"), 0o644); err != nil {
		t.Fatalf("write current: %v", err)
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/myapp/releases":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"tag_name": "v1.0.0",
					"assets": []map[string]string{
						{
							"name":                 "MyApp.AppImage",
							"browser_download_url": server.URL + "/assets/MyApp.AppImage",
						},
					},
				},
			})
		case "/assets/MyApp.AppImage":
			_, _ = w.Write([]byte("old release binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	generator := NewGenerator(Options{
		OutputDir:        outputDir,
		CacheDir:         cacheDir,
		FromVersions:     1,
		Source:           "github-release",
		TagPrefix:        "v",
		Repository:       "acme/myapp",
		GitHubAPIBaseURL: server.URL,
		HTTPClient:       server.Client(),
	})

	plan, err := generator.Plan()
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Pending) != 1 {
		t.Fatalf("expected 1 pending patch, got %d", len(plan.Pending))
	}
	if !strings.HasSuffix(plan.Pending[0].Patch, ".patch") {
		t.Fatalf("expected .patch output, got %q", plan.Pending[0].Patch)
	}
	if got := plan.Versions; len(got) != 1 || got[0] != "v1.0.0" {
		t.Fatalf("unexpected versions %v", got)
	}

	result, err := generator.Generate(plan)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(result.Generated) != 1 {
		t.Fatalf("expected 1 generated patch, got %d", len(result.Generated))
	}
	if !strings.HasSuffix(result.Generated[0].Patch, ".patch") {
		t.Fatalf("expected generated patch path to use .patch, got %q", result.Generated[0].Patch)
	}
	if _, err := os.Stat(result.ManifestPath); err != nil {
		t.Fatalf("expected manifest %s: %v", result.ManifestPath, err)
	}

	var manifest PatchManifest
	data, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if err := ValidateManifest(&manifest); err != nil {
		t.Fatalf("validate manifest: %v", err)
	}

	appliedPath := filepath.Join(root, "applied", "MyApp.AppImage")
	applied, err := Apply(filepath.Join(cacheDir, "v1.0.0", "linux", "amd64", "MyApp.AppImage"), result.Generated[0].Patch, appliedPath)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if "sha256:"+applied.Checksum != result.Generated[0].ToChecksum {
		t.Fatalf("unexpected applied checksum %s", applied.Checksum)
	}
}

func TestGeneratorFetchesURLManifestAssetsWithAuth(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "dist")
	cacheDir := filepath.Join(root, ".wailsrel", "cache")

	currentPath := filepath.Join(outputDir, "darwin", "arm64", "MyApp.app", "Contents", "MacOS", "MyApp")
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.WriteFile(currentPath, []byte("new bundle binary"), 0o755); err != nil {
		t.Fatalf("write current: %v", err)
	}

	archivePath := filepath.Join(root, "MyApp.app.zip")
	writeZipBundle(t, archivePath, "old bundle binary")

	var (
		sawManifestAuth bool
		sawAssetAuth    bool
		server          *httptest.Server
	)
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected manifest auth header %q", got)
			}
			sawManifestAuth = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"release": map[string]any{
					"tag": "v1.0.0",
				},
				"artifacts": []map[string]any{
					{
						"path":       "darwin/arm64/MyApp.app",
						"asset_name": "MyApp-1.0.0-darwin-arm64-app.zip",
						"url":        server.URL + "/assets/MyApp-1.0.0-darwin-arm64-app.zip",
						"checksum":   "sha256:deadbeef",
					},
				},
			})
		case "/assets/MyApp-1.0.0-darwin-arm64-app.zip":
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected asset auth header %q", got)
			}
			sawAssetAuth = true
			data, err := os.ReadFile(archivePath)
			if err != nil {
				t.Fatalf("read archive: %v", err)
			}
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	generator := NewGenerator(Options{
		OutputDir:    outputDir,
		CacheDir:     cacheDir,
		FromVersions: 1,
		Source:       "url",
		TagPrefix:    "v",
		ManifestURL:  server.URL + "/manifest.json",
		AuthToken:    "test-token",
		HTTPClient:   server.Client(),
	})

	plan, err := generator.Plan()
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !sawManifestAuth || !sawAssetAuth {
		t.Fatalf("expected auth headers on manifest and asset requests")
	}
	if len(plan.Pending) != 1 {
		t.Fatalf("expected 1 pending patch, got %d", len(plan.Pending))
	}
	if got := plan.Versions; len(got) != 1 || got[0] != "v1.0.0" {
		t.Fatalf("unexpected versions %v", got)
	}

	cachedBundle := filepath.Join(cacheDir, "v1.0.0", "darwin", "arm64", "MyApp.app", "Contents", "MacOS", "MyApp")
	data, err := os.ReadFile(cachedBundle)
	if err != nil {
		t.Fatalf("expected extracted bundle %s: %v", cachedBundle, err)
	}
	if string(data) != "old bundle binary" {
		t.Fatalf("unexpected cached bundle contents %q", string(data))
	}
}

func TestDiscoverCachedVersionsSortsAndLimits(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"v1.0.0", "v1.2.0", "v1.1.5", "notes"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	versions, err := discoverCachedVersions(root, "v", 2)
	if err != nil {
		t.Fatalf("discover versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].name != "v1.2.0" || versions[1].name != "v1.1.5" {
		t.Fatalf("unexpected versions order: %#v", versions)
	}
}

func writeAppBundle(t *testing.T, root, payload string) {
	t.Helper()

	binaryPath := filepath.Join(root, "Contents", "MacOS", "MyApp")
	resourcesPath := filepath.Join(root, "Contents", "Resources", "index.html")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatalf("mkdir binary dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(resourcesPath), 0o755); err != nil {
		t.Fatalf("mkdir resource dir: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte(payload+"-binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if err := os.WriteFile(resourcesPath, []byte("<html>"+strings.ToUpper(payload)+"</html>"), 0o644); err != nil {
		t.Fatalf("write resource: %v", err)
	}
}

func writeZipBundle(t *testing.T, archivePath, payload string) {
	t.Helper()

	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	entry, err := writer.Create("MyApp.app/Contents/MacOS/MyApp")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := entry.Write([]byte(payload)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

func deterministicBytes(seed int64, n int) []byte {
	rng := rand.New(rand.NewSource(seed))
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(rng.Intn(256))
	}
	return data
}
