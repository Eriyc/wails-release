package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/wailsrel/pkg/build"
	"github.com/you/wailsrel/pkg/delta"
	"github.com/you/wailsrel/pkg/frontend"
	"github.com/you/wailsrel/pkg/release"
)

func TestHTTPCheckerCheckReturnsNativeUpdateWithDelta(t *testing.T) {
	oldBinary := []byte("old-binary")
	newBinary := []byte("new-binary")
	newChecksum := "sha256:" + checksumHex(newBinary)
	oldChecksum := "sha256:" + checksumHex(oldBinary)

	artifactPath := runtime.GOOS + "/" + runtime.GOARCH + "/MyApp.bin"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(release.Manifest{
				SchemaVersion: 1,
				Release: release.ManifestRelease{
					Version: "2.0.0",
				},
				Artifacts: []release.ManifestArtifact{
					{
						Path:     artifactPath,
						OS:       runtime.GOOS,
						Arch:     runtime.GOARCH,
						Checksum: newChecksum,
						Size:     int64(len(newBinary)),
						URL:      "/download/app.bin",
					},
				},
				Delta: &release.ManifestDelta{
					ManifestURL: "/delta/manifest.json",
				},
			})
		case "/delta/manifest.json":
			_ = json.NewEncoder(w).Encode(delta.PatchManifest{
				SchemaVersion: 1,
				Algorithm:     "bsdiff",
				Patches: []delta.PatchManifestEntry{
					{
						Artifact:    artifactPath,
						Patch:       "/download/app.patch",
						FromSHA256:  trimSHA256(oldChecksum),
						ToSHA256:    trimSHA256(newChecksum),
						PatchSHA256: checksumHex([]byte("patch-bytes")),
						PatchSize:   int64(len("patch-bytes")),
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	checker := NewChecker(server.Client())
	result, err := checker.Check(context.Background(), CheckOpts{
		CurrentVersion: "1.0.0",
		CurrentHash:    oldChecksum,
		ManifestURL:    server.URL + "/manifest.json",
	})
	if err != nil {
		t.Fatalf("check for update: %v", err)
	}
	if !result.Available {
		t.Fatal("expected update to be available")
	}
	if result.Native == nil {
		t.Fatal("expected native update info")
	}
	if result.Native.Version != "2.0.0" {
		t.Fatalf("expected version 2.0.0, got %q", result.Native.Version)
	}
	if result.Native.ArtifactURL != server.URL+"/download/app.bin" {
		t.Fatalf("expected artifact url %q, got %q", server.URL+"/download/app.bin", result.Native.ArtifactURL)
	}
	if result.Native.DeltaURL != server.URL+"/download/app.patch" {
		t.Fatalf("expected delta url %q, got %q", server.URL+"/download/app.patch", result.Native.DeltaURL)
	}
	if result.Native.DeltaHash != "sha256:"+checksumHex([]byte("patch-bytes")) {
		t.Fatalf("expected delta hash to be populated, got %q", result.Native.DeltaHash)
	}
	if result.Native.DeltaFromHash != oldChecksum {
		t.Fatalf("expected delta from hash %q, got %q", oldChecksum, result.Native.DeltaFromHash)
	}
}

func TestHTTPCheckerCheckFiltersByChannelAndMarksMandatoryFromMetadata(t *testing.T) {
	stableBinary := []byte("stable-binary")
	betaBinary := []byte("beta-binary")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/manifest.json" {
			http.NotFound(w, r)
			return
		}

		_ = json.NewEncoder(w).Encode(release.Manifest{
			SchemaVersion: 1,
			Release: release.ManifestRelease{
				Version: "2.0.0",
			},
			Artifacts: []release.ManifestArtifact{
				{
					Path:     runtime.GOOS + "/" + runtime.GOARCH + "/MyApp-stable.bin",
					OS:       runtime.GOOS,
					Arch:     runtime.GOARCH,
					Checksum: "sha256:" + checksumHex(stableBinary),
					Size:     int64(len(stableBinary)),
					URL:      "/download/stable.bin",
					Metadata: map[string]string{
						"channel": "stable",
					},
				},
				{
					Path:     runtime.GOOS + "/" + runtime.GOARCH + "/MyApp-beta.bin",
					OS:       runtime.GOOS,
					Arch:     runtime.GOARCH,
					Checksum: "sha256:" + checksumHex(betaBinary),
					Size:     int64(len(betaBinary)),
					URL:      "/download/beta.bin",
					Metadata: map[string]string{
						"channel":               "beta",
						"mandatory_min_version": "1.5.0",
						"native_compat":         "2",
					},
				},
			},
		})
	}))
	defer server.Close()

	checker := NewChecker(server.Client())
	result, err := checker.Check(context.Background(), CheckOpts{
		CurrentVersion: "1.0.0",
		CurrentHash:    "sha256:" + checksumHex([]byte("current")),
		NativeCompat:   "2",
		Channel:        "beta",
		ManifestURL:    server.URL + "/manifest.json",
	})
	if err != nil {
		t.Fatalf("check for beta update: %v", err)
	}
	if result.Native == nil {
		t.Fatal("expected native beta update")
	}
	if result.Native.ArtifactURL != server.URL+"/download/beta.bin" {
		t.Fatalf("expected beta artifact url, got %q", result.Native.ArtifactURL)
	}
	if !result.Native.Mandatory {
		t.Fatal("expected update to be marked mandatory")
	}
}

func TestDefaultApplierApplyNativeFallsBackToFullWhenDeltaFails(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "MyApp.bin")
	oldBinary := []byte("old-binary")
	newBinary := []byte("new-binary")
	patchBytes := []byte("bad-patch")
	if err := os.WriteFile(targetPath, oldBinary, 0o755); err != nil {
		t.Fatalf("write current binary: %v", err)
	}

	var fullDownloads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download/full.bin":
			fullDownloads++
			_, _ = w.Write(newBinary)
		case "/download/update.patch":
			_, _ = w.Write(patchBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	applier := NewApplier(ApplierOptions{
		Client:     server.Client(),
		TargetPath: targetPath,
		TempDir:    filepath.Join(root, ".tmp"),
	})
	var deltaAttempts int
	applier.deltaApply = func(oldPath, patchPath, outPath string) error {
		deltaAttempts++
		return errors.New("delta failed")
	}

	var progressCalls int
	err := applier.ApplyNative(context.Background(), &UpdateInfo{
		Version:       "2.0.0",
		ArtifactURL:   server.URL + "/download/full.bin",
		ArtifactHash:  "sha256:" + checksumHex(newBinary),
		ArtifactSize:  int64(len(newBinary)),
		DeltaURL:      server.URL + "/download/update.patch",
		DeltaHash:     "sha256:" + checksumHex(patchBytes),
		DeltaSize:     int64(len(patchBytes)),
		DeltaFromHash: "sha256:" + checksumHex(oldBinary),
	}, func(downloaded, total int64) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("apply native update: %v", err)
	}
	if deltaAttempts != 1 {
		t.Fatalf("expected one delta attempt, got %d", deltaAttempts)
	}
	if fullDownloads != 1 {
		t.Fatalf("expected one full download, got %d", fullDownloads)
	}
	if progressCalls == 0 {
		t.Fatal("expected progress callback to be invoked")
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read updated binary: %v", err)
	}
	if string(data) != string(newBinary) {
		t.Fatalf("expected updated binary %q, got %q", string(newBinary), string(data))
	}
}

func TestDefaultApplierApplyNativeResumesFullDownload(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "MyApp.bin")
	if err := os.WriteFile(targetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write current binary: %v", err)
	}

	newBinary := []byte(strings.Repeat("abcdef", 256))
	partPath := filepath.Join(root, ".tmp", "full-download.bin.part")
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		t.Fatalf("mkdir temp dir: %v", err)
	}
	prefixLen := 173
	if err := os.WriteFile(partPath, newBinary[:prefixLen], 0o644); err != nil {
		t.Fatalf("write partial download: %v", err)
	}

	var rangeHeader atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/full.bin" {
			http.NotFound(w, r)
			return
		}
		rangeHeader.Store(r.Header.Get("Range"))

		start := 0
		if header := r.Header.Get("Range"); header != "" {
			if _, err := fmt.Sscanf(header, "bytes=%d-", &start); err != nil {
				t.Fatalf("parse range header %q: %v", header, err)
			}
			w.WriteHeader(http.StatusPartialContent)
		}
		_, _ = w.Write(newBinary[start:])
	}))
	defer server.Close()

	applier := NewApplier(ApplierOptions{
		Client:     server.Client(),
		TargetPath: targetPath,
		TempDir:    filepath.Join(root, ".tmp"),
	})

	err := applier.ApplyNative(context.Background(), &UpdateInfo{
		Version:      "2.0.0",
		ArtifactURL:  server.URL + "/download/full.bin",
		ArtifactHash: "sha256:" + checksumHex(newBinary),
		ArtifactSize: int64(len(newBinary)),
	}, nil)
	if err != nil {
		t.Fatalf("apply native update with resume: %v", err)
	}

	if got := rangeHeader.Load(); got == nil || got.(string) != fmt.Sprintf("bytes=%d-", prefixLen) {
		t.Fatalf("expected range resume header %q, got %v", fmt.Sprintf("bytes=%d-", prefixLen), got)
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target path: %v", err)
	}
	if string(data) != string(newBinary) {
		t.Fatal("expected resumed download to replace target with full binary")
	}
}

func TestDefaultApplierApplyNativeRollsBackOnVerificationFailure(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "MyApp.bin")
	oldBinary := []byte("old-binary")
	newBinary := []byte("new-binary")
	if err := os.WriteFile(targetPath, oldBinary, 0o755); err != nil {
		t.Fatalf("write current binary: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/full.bin" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(newBinary)
	}))
	defer server.Close()

	applier := NewApplier(ApplierOptions{
		Client:     server.Client(),
		TargetPath: targetPath,
		TempDir:    filepath.Join(root, ".tmp"),
	})
	applier.verifyInstalled = func(context.Context, string) error {
		return errors.New("new binary failed to start")
	}

	err := applier.ApplyNative(context.Background(), &UpdateInfo{
		Version:      "2.0.0",
		ArtifactURL:  server.URL + "/download/full.bin",
		ArtifactHash: "sha256:" + checksumHex(newBinary),
		ArtifactSize: int64(len(newBinary)),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "new binary failed to start") {
		t.Fatalf("expected post-replace verification error, got %v", err)
	}

	data, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		t.Fatalf("read rolled back binary: %v", readErr)
	}
	if string(data) != string(oldBinary) {
		t.Fatalf("expected rollback to restore %q, got %q", string(oldBinary), string(data))
	}
}

func TestDefaultApplierApplyFrontendInstallsBundle(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, "frontend", "dist")
	if err := os.MkdirAll(filepath.Join(buildDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir frontend dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<!doctype html><html><body>phase6</body></html>\n"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "assets", "app.js"), []byte("console.log('phase6');\n"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}

	bundlePath := filepath.Join(root, "frontend-beta-2.0.0.zip")
	artifact, err := frontend.BuildBundle(context.Background(), frontend.BundleOpts{
		WorkDir:       root,
		OutputDir:     root,
		OutputPath:    bundlePath,
		BuildCommand:  "go version",
		BuildDir:      "frontend/dist",
		CompatVer:     "2",
		CompatVersion: 2,
		Channel:       "beta",
		Version:       "2.0.0",
	})
	if err != nil {
		t.Fatalf("build frontend bundle fixture: %v", err)
	}
	bundleChecksum, err := build.ComputeChecksum(artifact.Path)
	if err != nil {
		t.Fatalf("compute bundle checksum: %v", err)
	}
	bundleBytes, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatalf("read bundle bytes: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/frontend-beta.zip" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(bundleBytes)
	}))
	defer server.Close()

	manager := &frontend.BundleManager{
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}
	if err := manager.SetChannel("beta"); err != nil {
		t.Fatalf("set active channel: %v", err)
	}

	applier := NewApplier(ApplierOptions{
		Client:          server.Client(),
		FrontendManager: manager,
		TempDir:         filepath.Join(root, ".tmp"),
	})
	err = applier.ApplyFrontend(context.Background(), &FrontendUpdateInfo{
		Channel: "beta",
		Version: "2.0.0",
		URL:     server.URL + "/frontend-beta.zip",
		Hash:    "sha256:" + bundleChecksum,
		Size:    int64(len(bundleBytes)),
	}, nil)
	if err != nil {
		t.Fatalf("apply frontend update: %v", err)
	}

	overrideFS, manifest, err := manager.LoadActive()
	if err != nil {
		t.Fatalf("load active frontend: %v", err)
	}
	if overrideFS == nil || manifest == nil {
		t.Fatal("expected active frontend bundle to be installed")
	}
	if manifest.Channel != "beta" {
		t.Fatalf("expected installed channel beta, got %q", manifest.Channel)
	}
	if manifest.Version != "2.0.0" {
		t.Fatalf("expected installed version 2.0.0, got %q", manifest.Version)
	}
}

func TestManagerApplyFrontendOnlyDoesNotRestart(t *testing.T) {
	var nativeCalls int
	var frontendCalls int
	var restartCalls int

	manager := NewManager(ManagerOpts{
		Applier: applierFuncs{
			applyNative: func(context.Context, *UpdateInfo, ProgressFunc) error {
				nativeCalls++
				return nil
			},
			applyFrontend: func(context.Context, *FrontendUpdateInfo, ProgressFunc) error {
				frontendCalls++
				return nil
			},
		},
		OnRestart: func() error {
			restartCalls++
			return nil
		},
		OnProgress: func(downloaded, total int64) {},
	})

	err := manager.Apply(context.Background(), &UpdateInfo{
		Version: "2.0.0",
		Frontend: &FrontendUpdateInfo{
			Channel: "beta",
			Version: "2.0.0",
			URL:     "https://example.com/frontend.zip",
		},
	})
	if err != nil {
		t.Fatalf("apply frontend-only update: %v", err)
	}
	if nativeCalls != 0 {
		t.Fatalf("expected no native apply calls, got %d", nativeCalls)
	}
	if frontendCalls != 1 {
		t.Fatalf("expected one frontend apply call, got %d", frontendCalls)
	}
	if restartCalls != 0 {
		t.Fatalf("expected no restart for frontend-only update, got %d", restartCalls)
	}
}

func TestManagerStartPollsUntilContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var checks int32
	manager := NewManager(ManagerOpts{
		Checker: checkerFunc(func(context.Context, CheckOpts) (*CheckResult, error) {
			if atomic.AddInt32(&checks, 1) >= 2 {
				cancel()
			}
			return &CheckResult{}, nil
		}),
		AutoCheck:     true,
		CheckInterval: 10 * time.Millisecond,
	})

	done := make(chan struct{})
	go func() {
		manager.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("manager start did not stop after context cancellation")
	}

	if atomic.LoadInt32(&checks) < 2 {
		t.Fatalf("expected repeated checks, got %d", checks)
	}
}

func checksumHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func trimSHA256(value string) string {
	const prefix = "sha256:"
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):]
	}
	return value
}

type checkerFunc func(context.Context, CheckOpts) (*CheckResult, error)

func (fn checkerFunc) Check(ctx context.Context, opts CheckOpts) (*CheckResult, error) {
	return fn(ctx, opts)
}

type applierFuncs struct {
	applyNative   func(context.Context, *UpdateInfo, ProgressFunc) error
	applyFrontend func(context.Context, *FrontendUpdateInfo, ProgressFunc) error
}

func (a applierFuncs) ApplyNative(ctx context.Context, info *UpdateInfo, progress ProgressFunc) error {
	if a.applyNative == nil {
		return nil
	}
	return a.applyNative(ctx, info, progress)
}

func (a applierFuncs) ApplyFrontend(ctx context.Context, info *FrontendUpdateInfo, progress ProgressFunc) error {
	if a.applyFrontend == nil {
		return nil
	}
	return a.applyFrontend(ctx, info, progress)
}
