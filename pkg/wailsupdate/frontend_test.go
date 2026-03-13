package wailsupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Eriyc/wailsrel/pkg/frontend"
)

func TestRefreshFrontendCatalogForcedCodepushRetriesOnlyOnce(t *testing.T) {
	root := t.TempDir()
	manager := &frontend.BundleManager{
		AppID:        "com.example.app",
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	var bundleDownloads int
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/frontend/catalog.json":
			catalog := frontend.Catalog{
				SchemaVersion: frontend.CatalogSchemaVersion,
				AppID:         "com.example.app",
				GeneratedAt:   time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
				Codepush: []frontend.CodepushEntry{
					{
						Name:        "hotfix-1",
						Version:     "1.0.1",
						CompatID:    "2",
						URL:         server.URL + "/download/codepush.zip",
						Checksum:    "sha256:" + strings.Repeat("a", 64),
						Size:        99,
						Force:       true,
						PublishedAt: time.Date(2026, 3, 13, 12, 30, 0, 0, time.UTC),
					},
				},
			}
			writeSignedCatalog(t, w, catalog, privateKey)
		case "/download/codepush.zip":
			bundleDownloads++
			_, _ = w.Write([]byte("not-a-zip"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService(Options{
		CurrentVersion:            "1.0.0",
		TargetPath:                writeTempFile(t, "binary-data"),
		Client:                    server.Client(),
		FrontendManager:           manager,
		FrontendCatalogURL:        server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey:  base64.StdEncoding.EncodeToString(publicKey),
	})

	state := service.RefreshFrontendCatalog()
	if !state.Stale || state.LastError == "" {
		t.Fatalf("expected forced codepush failure to mark stale state, got %+v", state)
	}
	failed, err := manager.HasForcedCodepushFailure("1.0.1")
	if err != nil {
		t.Fatalf("load forced failures: %v", err)
	}
	if !failed {
		t.Fatal("expected forced codepush failure to be recorded")
	}

	service.RefreshFrontendCatalog()
	if bundleDownloads != 1 {
		t.Fatalf("expected one forced codepush download attempt, got %d", bundleDownloads)
	}
}

func TestRefreshFrontendCatalogFailurePreservesSelection(t *testing.T) {
	root := t.TempDir()
	manager := &frontend.BundleManager{
		AppID:        "com.example.app",
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	experimentPath := buildFrontendBundle(t, root, frontend.BundleManifest{
		Kind:          frontend.BundleKindExperiment,
		Name:          "green",
		Version:       "1.0.0",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>green</title>\n",
	})
	experimentFile, err := os.Open(experimentPath)
	if err != nil {
		t.Fatalf("open experiment bundle: %v", err)
	}
	defer experimentFile.Close()
	if err := manager.InstallExperiment(context.Background(), "green", experimentFile); err != nil {
		t.Fatalf("install experiment: %v", err)
	}
	if err := manager.SetExperiment("green"); err != nil {
		t.Fatalf("set experiment: %v", err)
	}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusBadGateway)
	}))
	defer server.Close()

	service := NewService(Options{
		CurrentVersion:            "1.0.0",
		TargetPath:                writeTempFile(t, "binary-data"),
		Client:                    server.Client(),
		FrontendManager:           manager,
		FrontendCatalogURL:        server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey:  base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)),
	})

	state := service.RefreshFrontendCatalog()
	if state.Selection != "green" {
		t.Fatalf("expected selection to persist on catalog fetch failure, got %+v", state)
	}
	if state.ActiveMode != frontend.BundleKindExperiment {
		t.Fatalf("expected experiment to remain active, got %+v", state)
	}
}

func TestSwitchExperimentInstallsAndSelectsBundle(t *testing.T) {
	root := t.TempDir()
	manager := &frontend.BundleManager{
		AppID:        "com.example.app",
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	bundlePath := buildFrontendBundle(t, root, frontend.BundleManifest{
		Kind:          frontend.BundleKindExperiment,
		Name:          "green",
		Version:       "1.0.0",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>green</title>\n",
	})
	bundleBytes, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	checksum, err := frontendBundleChecksum(bundlePath)
	if err != nil {
		t.Fatalf("bundle checksum: %v", err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/frontend/catalog.json":
			catalog := frontend.Catalog{
				SchemaVersion: frontend.CatalogSchemaVersion,
				AppID:         "com.example.app",
				GeneratedAt:   time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
				Experiments: []frontend.ExperimentEntry{
					{
						Name:        "green",
						Version:     "1.0.0",
						CompatID:    "2",
						URL:         server.URL + "/download/green.zip",
						Checksum:    checksum,
						Size:        int64(len(bundleBytes)),
						DisplayName: "Green",
						PublishedAt: time.Date(2026, 3, 13, 12, 30, 0, 0, time.UTC),
					},
				},
			}
			writeSignedCatalog(t, w, catalog, privateKey)
		case "/download/green.zip":
			_, _ = w.Write(bundleBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService(Options{
		CurrentVersion:            "1.0.0",
		TargetPath:                writeTempFile(t, "binary-data"),
		Client:                    server.Client(),
		FrontendManager:           manager,
		FrontendCatalogURL:        server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey:  base64.StdEncoding.EncodeToString(publicKey),
	})

	service.RefreshFrontendCatalog()
	result := service.SwitchExperiment("green")
	if result.Error != "" || !result.Applied {
		t.Fatalf("expected experiment switch success, got %+v", result)
	}
	if selection, _ := manager.GetSelection(); selection != "green" {
		t.Fatalf("expected selected experiment green, got %q", selection)
	}
	manifest, err := manager.LoadInstalledExperiment("green")
	if err != nil || manifest == nil {
		t.Fatalf("expected installed experiment, manifest=%+v err=%v", manifest, err)
	}
}

func TestRefreshFrontendCatalogClearsRemovedSelection(t *testing.T) {
	root := t.TempDir()
	manager := &frontend.BundleManager{
		AppID:        "com.example.app",
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	experimentPath := buildFrontendBundle(t, root, frontend.BundleManifest{
		Kind:          frontend.BundleKindExperiment,
		Name:          "green",
		Version:       "1.0.0",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>green</title>\n",
	})
	experimentFile, err := os.Open(experimentPath)
	if err != nil {
		t.Fatalf("open experiment bundle: %v", err)
	}
	defer experimentFile.Close()
	if err := manager.InstallExperiment(context.Background(), "green", experimentFile); err != nil {
		t.Fatalf("install experiment: %v", err)
	}
	if err := manager.SetExperiment("green"); err != nil {
		t.Fatalf("set experiment: %v", err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		catalog := frontend.Catalog{
			SchemaVersion: frontend.CatalogSchemaVersion,
			AppID:         "com.example.app",
			GeneratedAt:   time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		}
		writeSignedCatalog(t, w, catalog, privateKey)
	}))
	defer server.Close()

	service := NewService(Options{
		CurrentVersion:            "1.0.0",
		TargetPath:                writeTempFile(t, "binary-data"),
		Client:                    server.Client(),
		FrontendManager:           manager,
		FrontendCatalogURL:        server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey:  base64.StdEncoding.EncodeToString(publicKey),
	})

	state := service.RefreshFrontendCatalog()
	if state.Selection != "" {
		t.Fatalf("expected removed selection to be cleared, got %+v", state)
	}
	if state.ActiveMode != "embedded" {
		t.Fatalf("expected fallback to embedded, got %+v", state)
	}
}

func buildFrontendBundle(t *testing.T, root string, manifest frontend.BundleManifest, files map[string]string) string {
	t.Helper()

	buildDir := filepath.Join(root, manifest.Name+"-build")
	for relativePath, contents := range files {
		fullPath := filepath.Join(buildDir, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", relativePath, err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", relativePath, err)
		}
	}

	outputPath := filepath.Join(root, manifest.Name+".zip")
	artifact, err := frontend.BuildBundle(context.Background(), frontend.BundleOpts{
		WorkDir:       root,
		OutputPath:    outputPath,
		OutputDir:     root,
		BuildCommand:  "go version",
		BuildDir:      filepath.Base(buildDir),
		CompatVer:     manifest.CompatID,
		CompatVersion: manifest.CompatVersion,
		Channel:       manifest.Channel,
		Kind:          manifest.Kind,
		Name:          manifest.Name,
		SourceBranch:  manifest.SourceBranch,
		CommitSHA:     manifest.CommitSHA,
		Version:       manifest.Version,
	})
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	return artifact.Path
}

func frontendBundleChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256Sum(data)
	return "sha256:" + sum, nil
}

func writeSignedCatalog(t *testing.T, w http.ResponseWriter, catalog frontend.Catalog, privateKey ed25519.PrivateKey) {
	t.Helper()
	copy := catalog
	copy.Signature = ""
	payload, err := json.Marshal(copy)
	if err != nil {
		t.Fatalf("marshal catalog payload: %v", err)
	}
	catalog.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	if err := json.NewEncoder(w).Encode(catalog); err != nil {
		t.Fatalf("encode catalog: %v", err)
	}
}

func sha256Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
