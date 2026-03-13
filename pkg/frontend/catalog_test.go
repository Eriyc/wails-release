package frontend

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecodeCatalogVerifiesSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	catalog := Catalog{
		SchemaVersion: CatalogSchemaVersion,
		AppID:         "com.example.app",
		GeneratedAt:   time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		Codepush: []CodepushEntry{
			{
				Name:        "hotfix-1",
				Version:     "1.2.4",
				CompatID:    "2",
				URL:         "https://proxy.example.com/frontend/codepush.zip",
				Checksum:    "sha256:" + strings.Repeat("a", 64),
				Size:        128,
				Force:       true,
				PublishedAt: time.Date(2026, 3, 13, 12, 30, 0, 0, time.UTC),
			},
		},
	}
	signCatalog(t, &catalog, privateKey)

	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	decoded, err := DecodeCatalog(data, base64.StdEncoding.EncodeToString(publicKey), "com.example.app")
	if err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if decoded.AppID != catalog.AppID {
		t.Fatalf("expected app id %q, got %q", catalog.AppID, decoded.AppID)
	}

	catalog.Codepush[0].URL = "https://proxy.example.com/frontend/tampered.zip"
	data, err = json.Marshal(catalog)
	if err != nil {
		t.Fatalf("marshal tampered catalog: %v", err)
	}
	if _, err := DecodeCatalog(data, base64.StdEncoding.EncodeToString(publicKey), "com.example.app"); err == nil {
		t.Fatal("expected signature verification failure after tampering")
	}
}

func TestValidateVariantNameRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../x", "a/b", "%2e%2e"} {
		if err := ValidateVariantName(name); err == nil {
			t.Fatalf("expected invalid name %q to be rejected", name)
		}
	}
	if err := ValidateVariantName("good-name_1"); err != nil {
		t.Fatalf("expected valid name, got %v", err)
	}
}

func TestSelectNewestCodepushPrefersPublishedAtThenSemver(t *testing.T) {
	selected := SelectNewestCodepush([]CodepushEntry{
		{
			Name:        "alpha",
			Version:     "1.2.3",
			URL:         "https://proxy.example.com/a.zip",
			Checksum:    "sha256:" + strings.Repeat("a", 64),
			PublishedAt: time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		},
		{
			Name:        "beta",
			Version:     "1.2.5",
			URL:         "https://proxy.example.com/b.zip",
			Checksum:    "sha256:" + strings.Repeat("b", 64),
			PublishedAt: time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		},
		{
			Name:        "gamma",
			Version:     "1.2.4",
			URL:         "https://proxy.example.com/c.zip",
			Checksum:    "sha256:" + strings.Repeat("c", 64),
			PublishedAt: time.Date(2026, 3, 13, 12, 5, 0, 0, time.UTC),
		},
	}, "")
	if selected == nil || selected.Name != "gamma" {
		t.Fatalf("expected newest published codepush gamma, got %+v", selected)
	}

	selected = SelectNewestCodepush([]CodepushEntry{
		{
			Name:        "alpha",
			Version:     "1.2.3",
			URL:         "https://proxy.example.com/a.zip",
			Checksum:    "sha256:" + strings.Repeat("a", 64),
			PublishedAt: time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		},
		{
			Name:        "beta",
			Version:     "1.2.5",
			URL:         "https://proxy.example.com/b.zip",
			Checksum:    "sha256:" + strings.Repeat("b", 64),
			PublishedAt: time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		},
	}, "")
	if selected == nil || selected.Name != "beta" {
		t.Fatalf("expected highest semver tie-breaker beta, got %+v", selected)
	}
}

func TestBundleManagerLoadEffectiveClearsInvalidSelectionAndFallsBackToCodepush(t *testing.T) {
	root := t.TempDir()
	manager := &BundleManager{
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	experimentBundle := writeTestBundleArchive(t, root, "experiment-one", BundleManifest{
		Kind:          BundleKindExperiment,
		Name:          "green",
		Version:       "1.0.0",
		CompatID:      "2",
		CompatVersion: 2,
		Checksum:      "sha256:" + strings.Repeat("d", 64),
	}, map[string]string{
		"index.html": "<title>green</title>\n",
	})
	experimentFile, err := os.Open(experimentBundle)
	if err != nil {
		t.Fatalf("open experiment bundle: %v", err)
	}
	defer experimentFile.Close()
	if err := manager.InstallExperiment(context.Background(), "green", experimentFile); err == nil {
		t.Fatal("expected manifest checksum mismatch to reject the experiment fixture")
	}

	codepushManifest := BundleManifest{
		Kind:          BundleKindCodepush,
		Name:          "hotfix-1",
		Version:       "1.0.1",
		CompatID:      "2",
		CompatVersion: 2,
	}
	codepushPath := writeTestBundleArchive(t, root, "codepush", codepushManifest, map[string]string{
		"index.html": "<title>codepush</title>\n",
	})
	rewriteBundleChecksum(t, codepushPath)
	codepushFile, err := os.Open(codepushPath)
	if err != nil {
		t.Fatalf("open codepush bundle: %v", err)
	}
	defer codepushFile.Close()
	if err := manager.InstallCodepush(context.Background(), codepushFile); err != nil {
		t.Fatalf("install codepush: %v", err)
	}

	if err := os.WriteFile(manager.selectionPath(), []byte("{\"experiment\":\"green\"}\n"), 0o644); err != nil {
		t.Fatalf("write selection: %v", err)
	}

	filesystem, manifest, err := manager.LoadEffective()
	if err != nil {
		t.Fatalf("load effective: %v", err)
	}
	if manifest == nil || manifest.Kind != BundleKindCodepush {
		t.Fatalf("expected codepush fallback, got %+v", manifest)
	}
	index, err := fs.ReadFile(filesystem, "index.html")
	if err != nil {
		t.Fatalf("read effective index: %v", err)
	}
	if !strings.Contains(string(index), "codepush") {
		t.Fatalf("expected codepush contents, got %q", index)
	}
	if selection, _ := manager.GetSelection(); selection != "" {
		t.Fatalf("expected invalid selection to be cleared, got %q", selection)
	}
}

func TestBundleManagerRejectsTamperedInstalledManifest(t *testing.T) {
	root := t.TempDir()
	manager := &BundleManager{
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	bundleManifest := BundleManifest{
		Kind:          BundleKindCodepush,
		Name:          "hotfix-1",
		Version:       "1.0.1",
		CompatID:      "2",
		CompatVersion: 2,
	}
	bundlePath := writeTestBundleArchive(t, root, "codepush", bundleManifest, map[string]string{
		"index.html": "<title>codepush</title>\n",
	})
	rewriteBundleChecksum(t, bundlePath)
	file, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer file.Close()
	if err := manager.InstallCodepush(context.Background(), file); err != nil {
		t.Fatalf("install codepush: %v", err)
	}

	installedManifest, err := loadBundleManifest(filepath.Join(manager.codepushCurrentDir(), "bundle.json"))
	if err != nil {
		t.Fatalf("load installed manifest: %v", err)
	}
	installedManifest.Checksum = "sha256:" + strings.Repeat("f", 64)
	if err := writeBundleManifest(filepath.Join(manager.codepushCurrentDir(), "bundle.json"), *installedManifest); err != nil {
		t.Fatalf("tamper manifest: %v", err)
	}

	filesystem, manifest, err := manager.LoadEffective()
	if err != nil {
		t.Fatalf("load effective after tamper: %v", err)
	}
	if filesystem != nil || manifest != nil {
		t.Fatalf("expected tampered bundle to be ignored, got %+v", manifest)
	}
}

func signCatalog(t *testing.T, catalog *Catalog, privateKey ed25519.PrivateKey) {
	t.Helper()
	payload, err := catalog.signedPayload()
	if err != nil {
		t.Fatalf("signed payload: %v", err)
	}
	catalog.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
}

func rewriteBundleChecksum(t *testing.T, bundlePath string) {
	t.Helper()

	tempDir := t.TempDir()
	if err := extractBundleArchive(bundlePath, tempDir); err != nil {
		t.Fatalf("extract bundle: %v", err)
	}
	checksum, err := computeBundleDirectoryChecksum(tempDir)
	if err != nil {
		t.Fatalf("compute bundle checksum: %v", err)
	}
	manifest, err := loadBundleManifest(filepath.Join(tempDir, "bundle.json"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	manifest.Checksum = "sha256:" + checksum
	if err := writeBundleManifest(filepath.Join(tempDir, "bundle.json"), *manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := writeBundleArchive(bundlePath, tempDir, *manifest); err != nil {
		t.Fatalf("rewrite archive: %v", err)
	}
}
