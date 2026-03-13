package wailsupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Eriyc/wailsrel/pkg/frontend"
)

type frontendRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn frontendRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

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
		CurrentVersion:           "1.0.0",
		TargetPath:               writeTempFile(t, "binary-data"),
		Client:                   server.Client(),
		FrontendManager:          manager,
		FrontendCatalogURL:       server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey: base64.StdEncoding.EncodeToString(publicKey),
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

func TestRefreshFrontendCatalogForcedCodepushMarkerReadFailureMarksStateStale(t *testing.T) {
	root := t.TempDir()
	manager := &frontend.BundleManager{
		AppID:        "com.example.app",
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}
	if err := os.MkdirAll(manager.OverrideRoot, 0o755); err != nil {
		t.Fatalf("mkdir override root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manager.OverrideRoot, "forced-failures.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatalf("write malformed forced failure state: %v", err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	}))
	defer server.Close()

	service := NewService(Options{
		CurrentVersion:           "1.0.0",
		TargetPath:               writeTempFile(t, "binary-data"),
		Client:                   server.Client(),
		FrontendManager:          manager,
		FrontendCatalogURL:       server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey: base64.StdEncoding.EncodeToString(publicKey),
	})

	state := service.RefreshFrontendCatalog()
	if !state.Stale {
		t.Fatalf("expected stale state after forced failure marker read error, got %+v", state)
	}
	if !strings.Contains(state.LastError, "JSON") && !strings.Contains(state.LastError, "json") {
		t.Fatalf("expected malformed forced failure state error, got %+v", state)
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
		CurrentVersion:           "1.0.0",
		TargetPath:               writeTempFile(t, "binary-data"),
		Client:                   server.Client(),
		FrontendManager:          manager,
		FrontendCatalogURL:       server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey: base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)),
	})

	state := service.RefreshFrontendCatalog()
	if state.Selection != "green" {
		t.Fatalf("expected selection to persist on catalog fetch failure, got %+v", state)
	}
	if state.ActiveMode != frontend.BundleKindExperiment {
		t.Fatalf("expected experiment to remain active, got %+v", state)
	}
}

func TestRefreshFrontendCatalogRejectsRedirectedOrigin(t *testing.T) {
	manager := &frontend.BundleManager{
		AppID:        "com.example.app",
		NativeCompat: "2",
		OverrideRoot: filepath.Join(t.TempDir(), ".wailsrel"),
	}

	client := &http.Client{
		Transport: frontendRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			redirected := req.Clone(req.Context())
			redirected.URL.Scheme = "https"
			redirected.URL.Host = "mirror.example.com"
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`{}`)),
				Header:     make(http.Header),
				Request:    redirected,
			}, nil
		}),
	}

	service := NewService(Options{
		CurrentVersion:           "1.0.0",
		TargetPath:               writeTempFile(t, "binary-data"),
		Client:                   client,
		FrontendManager:          manager,
		FrontendCatalogURL:       "https://updates.example.com/frontend/catalog.json",
		FrontendCatalogPublicKey: base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)),
	})

	state := service.RefreshFrontendCatalog()
	if !state.Stale {
		t.Fatalf("expected stale state after redirected catalog fetch, got %+v", state)
	}
	if !strings.Contains(state.LastError, "redirected away from pinned origin") {
		t.Fatalf("expected redirected origin error, got %+v", state)
	}
}

func TestApplyCodepushWithoutCompatibleCodepushReturnsNoop(t *testing.T) {
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
		CurrentVersion:           "1.0.0",
		TargetPath:               writeTempFile(t, "binary-data"),
		Client:                   server.Client(),
		FrontendManager:          manager,
		FrontendCatalogURL:       server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey: base64.StdEncoding.EncodeToString(publicKey),
	})

	result := service.ApplyCodepush()
	if result.Applied || result.Error != "" {
		t.Fatalf("expected no-op codepush result, got %+v", result)
	}
	if result.Message != "There is no compatible codepush available." {
		t.Fatalf("unexpected no-op message: %+v", result)
	}
}

func TestApplyCodepushFailureMarksStateStale(t *testing.T) {
	t.Run("download status", func(t *testing.T) {
		service, _ := newCodepushApplyTestService(t, "sha256:"+strings.Repeat("a", 64), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}))

		result := service.ApplyCodepush()
		if result.Applied {
			t.Fatalf("expected apply failure, got %+v", result)
		}
		if result.Message != "Codepush apply failed." || !strings.Contains(result.Error, "unexpected status 502 Bad Gateway") {
			t.Fatalf("unexpected download failure result: %+v", result)
		}

		state := service.GetFrontendState()
		if !state.Stale || !strings.Contains(state.LastError, "unexpected status 502 Bad Gateway") {
			t.Fatalf("expected stale state after download failure, got %+v", state)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		service, _ := newCodepushApplyTestService(t, "sha256:"+strings.Repeat("f", 64), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-a-real-bundle"))
		}))

		result := service.ApplyCodepush()
		if result.Applied {
			t.Fatalf("expected apply failure, got %+v", result)
		}
		if result.Message != "Codepush apply failed." || !strings.Contains(result.Error, "checksum mismatch") {
			t.Fatalf("unexpected checksum failure result: %+v", result)
		}

		state := service.GetFrontendState()
		if !state.Stale || !strings.Contains(state.LastError, "checksum mismatch") {
			t.Fatalf("expected stale state after checksum mismatch, got %+v", state)
		}
	})
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
		CurrentVersion:           "1.0.0",
		TargetPath:               writeTempFile(t, "binary-data"),
		Client:                   server.Client(),
		FrontendManager:          manager,
		FrontendCatalogURL:       server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey: base64.StdEncoding.EncodeToString(publicKey),
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
		CurrentVersion:           "1.0.0",
		TargetPath:               writeTempFile(t, "binary-data"),
		Client:                   server.Client(),
		FrontendManager:          manager,
		FrontendCatalogURL:       server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey: base64.StdEncoding.EncodeToString(publicKey),
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

func newCodepushApplyTestService(t *testing.T, checksum string, bundleHandler http.Handler) (*Service, *frontend.BundleManager) {
	t.Helper()

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
						Checksum:    checksum,
						Size:        99,
						PublishedAt: time.Date(2026, 3, 13, 12, 30, 0, 0, time.UTC),
					},
				},
			}
			writeSignedCatalog(t, w, catalog, privateKey)
		case "/download/codepush.zip":
			bundleHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	service := NewService(Options{
		CurrentVersion:           "1.0.0",
		TargetPath:               writeTempFile(t, "binary-data"),
		Client:                   server.Client(),
		FrontendManager:          manager,
		FrontendCatalogURL:       server.URL + "/frontend/catalog.json",
		FrontendCatalogPublicKey: base64.StdEncoding.EncodeToString(publicKey),
	})
	return service, manager
}

func sha256Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
