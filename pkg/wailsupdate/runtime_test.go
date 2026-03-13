package wailsupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Eriyc/wailsrel/pkg/frontend"
)

func TestManifestURLFromBaseURL(t *testing.T) {
	got := ManifestURLFromBaseURL("https://updates.example.com/")
	if got != "https://updates.example.com/manifest" {
		t.Fatalf("unexpected manifest URL %q", got)
	}
}

func TestFrontendCatalogURLFromBaseURL(t *testing.T) {
	got := FrontendCatalogURLFromBaseURL("https://updates.example.com")
	if got != "https://updates.example.com/frontend/catalog" {
		t.Fatalf("unexpected catalog URL %q", got)
	}
}

func TestNewRuntimeDerivesBaseURLsWhenFrontendEnabled(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		AppID:          "com.example.myapp",
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			BaseURL: "https://updates.example.com",
		},
		Frontend: RuntimeFrontend{
			Enabled:          true,
			CatalogPublicKey: "test-public-key",
		},
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	service := runtime.Service()
	if service.opts.ManifestURL != "https://updates.example.com/manifest" {
		t.Fatalf("unexpected manifest URL %q", service.opts.ManifestURL)
	}
	if service.opts.FrontendCatalogURL != "https://updates.example.com/frontend/catalog" {
		t.Fatalf("unexpected frontend catalog URL %q", service.opts.FrontendCatalogURL)
	}
}

func TestNewRuntimeExplicitOverridesWin(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		AppID:          "com.example.myapp",
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			BaseURL:     "https://updates.example.com",
			ManifestURL: "https://cdn.example.com/custom-manifest",
		},
		Frontend: RuntimeFrontend{
			Enabled:          true,
			CatalogURL:       "https://cdn.example.com/custom-catalog",
			CatalogPublicKey: "test-public-key",
		},
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	service := runtime.Service()
	if service.opts.ManifestURL != "https://cdn.example.com/custom-manifest" {
		t.Fatalf("unexpected manifest override %q", service.opts.ManifestURL)
	}
	if service.opts.FrontendCatalogURL != "https://cdn.example.com/custom-catalog" {
		t.Fatalf("unexpected catalog override %q", service.opts.FrontendCatalogURL)
	}
}

func TestNewRuntimeRepositoryModeDerivesGitHubManifest(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			Repository: "owner/repo",
		},
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if got := runtime.Service().opts.ManifestURL; got != "https://github.com/owner/repo/releases/latest/download/manifest.json" {
		t.Fatalf("unexpected repository manifest URL %q", got)
	}
}

func TestNewRuntimeRepositoryModeRequiresExplicitFrontendCatalog(t *testing.T) {
	_, err := NewRuntime(RuntimeOptions{
		AppID:          "com.example.myapp",
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			Repository: "owner/repo",
		},
		Frontend: RuntimeFrontend{
			Enabled:          true,
			CatalogPublicKey: "test-public-key",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "frontend catalog URL is required") {
		t.Fatalf("expected frontend catalog URL error, got %v", err)
	}
}

func TestNewRuntimeRejectsBaseURLAndRepositoryTogether(t *testing.T) {
	_, err := NewRuntime(RuntimeOptions{
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			BaseURL:    "https://updates.example.com",
			Repository: "owner/repo",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot both be set") {
		t.Fatalf("expected union error, got %v", err)
	}
}

func TestNewRuntimeRequiresAppIDWhenFrontendEnabled(t *testing.T) {
	_, err := NewRuntime(RuntimeOptions{
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			BaseURL: "https://updates.example.com",
		},
		Frontend: RuntimeFrontend{
			Enabled: true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "frontend app ID is required") {
		t.Fatalf("expected app ID error, got %v", err)
	}
}

func TestNewRuntimeLeavesFrontendDisabledByDefault(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			BaseURL: "https://updates.example.com",
		},
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if runtime.FrontendManager() != nil {
		t.Fatal("expected frontend manager to be nil by default")
	}
	if runtime.Service().opts.FrontendCatalogURL != "" {
		t.Fatalf("expected no frontend catalog URL, got %q", runtime.Service().opts.FrontendCatalogURL)
	}
}

func TestNewRuntimeCreatesBearerWrappedClient(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			BaseURL: "https://updates.example.com",
		},
		BearerToken: "token-123",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if runtime.Service().opts.Client.Timeout != defaultRuntimeClientTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultRuntimeClientTimeout, runtime.Service().opts.Client.Timeout)
	}
	if _, ok := runtime.Service().opts.Client.Transport.(bearerTransport); !ok {
		t.Fatalf("expected bearer transport, got %T", runtime.Service().opts.Client.Transport)
	}
}

func TestNewRuntimeClonesProvidedClientBeforeWrappingBearerTransport(t *testing.T) {
	transport := &captureTransport{}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	runtime, err := NewRuntime(RuntimeOptions{
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			BaseURL: "https://updates.example.com",
		},
		Client:      client,
		BearerToken: "token-123",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if runtime.Service().opts.Client == client {
		t.Fatal("expected provided client to be cloned")
	}
	if runtime.Service().opts.Client.Timeout != client.Timeout {
		t.Fatalf("expected cloned timeout %s, got %s", client.Timeout, runtime.Service().opts.Client.Timeout)
	}
	if runtime.Service().opts.Client.Transport == client.Transport {
		t.Fatal("expected transport wrapper around provided transport")
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://updates.example.com/manifest", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if _, err := runtime.Service().opts.Client.Transport.RoundTrip(req); err != nil {
		t.Fatalf("wrapped round trip: %v", err)
	}
	if transport.auth != "Bearer token-123" {
		t.Fatalf("expected bearer auth header, got %q", transport.auth)
	}
}

func TestRuntimeAssetFSFallsBackToEmbeddedAssetsWhenFrontendDisabled(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			BaseURL: "https://updates.example.com",
		},
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	data, err := fs.ReadFile(runtime.AssetFS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("embedded")},
	}), "index.html")
	if err != nil {
		t.Fatalf("read embedded asset: %v", err)
	}
	if string(data) != "embedded" {
		t.Fatalf("expected embedded asset, got %q", string(data))
	}
}

func TestRuntimeAssetFSUsesInstalledFrontendBundleWhenAvailable(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		AppID:          "com.example.myapp",
		CurrentVersion: "1.0.0",
		NativeCompat:   "2",
		Source: RuntimeSource{
			BaseURL: "https://updates.example.com",
		},
		Frontend: RuntimeFrontend{
			Enabled:          true,
			CatalogPublicKey: "test-public-key",
		},
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	manager := runtime.FrontendManager()
	manager.OverrideRoot = filepath.Join(t.TempDir(), ".wailsrel")
	bundlePath := writeRuntimeTestBundleArchive(t, BundleSpec{
		Kind:       frontend.BundleKindCodepush,
		Name:       "default",
		Version:    "1.2.3",
		CompatID:   "2",
		Channel:    "stable",
		FilePath:   "index.html",
		FileData:   "override",
		FileMode:   0o644,
		OutputPath: filepath.Join(t.TempDir(), "codepush.zip"),
	})

	file, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle archive: %v", err)
	}
	defer file.Close()

	if err := manager.InstallCodepush(context.Background(), file); err != nil {
		t.Fatalf("install codepush: %v", err)
	}

	data, err := fs.ReadFile(runtime.AssetFS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("embedded")},
	}), "index.html")
	if err != nil {
		t.Fatalf("read runtime asset: %v", err)
	}
	if string(data) != "override" {
		t.Fatalf("expected override asset, got %q", string(data))
	}
}

func TestRuntimeAssetFSUsesEmbeddedAssetsWithoutInstalledBundle(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		AppID:          "com.example.myapp",
		CurrentVersion: "1.0.0",
		Source: RuntimeSource{
			BaseURL: "https://updates.example.com",
		},
		Frontend: RuntimeFrontend{
			Enabled:          true,
			CatalogPublicKey: "test-public-key",
		},
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	data, err := fs.ReadFile(runtime.AssetFS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("embedded")},
	}), "index.html")
	if err != nil {
		t.Fatalf("read runtime asset: %v", err)
	}
	if string(data) != "embedded" {
		t.Fatalf("expected embedded asset, got %q", string(data))
	}
}

type captureTransport struct {
	auth string
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.auth = req.Header.Get("Authorization")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
		Request:    req,
	}, nil
}

type BundleSpec struct {
	Kind       string
	Name       string
	Version    string
	CompatID   string
	Channel    string
	FilePath   string
	FileData   string
	FileMode   fs.FileMode
	OutputPath string
}

func writeRuntimeTestBundleArchive(t *testing.T, spec BundleSpec) string {
	t.Helper()

	checksum := runtimeTestBundleChecksum(spec.FilePath, spec.FileMode, spec.FileData)
	manifest := frontend.BundleManifest{
		Version:       spec.Version,
		CompatID:      spec.CompatID,
		CompatVersion: 2,
		Kind:          spec.Kind,
		Name:          spec.Name,
		Channel:       spec.Channel,
		Checksum:      "sha256:" + checksum,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}

	file, err := os.Create(spec.OutputPath)
	if err != nil {
		t.Fatalf("create bundle archive: %v", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	manifestData = append(manifestData, '\n')
	writeRuntimeZipFile(t, zipWriter, "bundle.json", manifestData, 0o644)
	writeRuntimeZipFile(t, zipWriter, spec.FilePath, []byte(spec.FileData), spec.FileMode)

	return spec.OutputPath
}

func writeRuntimeZipFile(t *testing.T, zw *zip.Writer, name string, data []byte, mode fs.FileMode) {
	t.Helper()

	header := &zip.FileHeader{
		Name:   filepath.ToSlash(name),
		Method: zip.Deflate,
	}
	header.SetMode(mode)
	writer, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatalf("create zip header %s: %v", name, err)
	}
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("write zip data %s: %v", name, err)
	}
}

func runtimeTestBundleChecksum(path string, mode fs.FileMode, data string) string {
	hash := sha256.New()
	paths := []string{path}
	sort.Strings(paths)
	for _, current := range paths {
		_, _ = io.WriteString(hash, filepath.ToSlash(current))
		_, _ = io.WriteString(hash, "\n")
		_, _ = io.WriteString(hash, mode.String())
		_, _ = io.WriteString(hash, "\n")
		_, _ = io.WriteString(hash, data)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
