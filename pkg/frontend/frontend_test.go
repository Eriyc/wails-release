package frontend

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLayeredFSPrefersOverrideWhenPresent(t *testing.T) {
	layered := NewLayeredFS(
		fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("override index")},
		},
		fstest.MapFS{
			"index.html":    &fstest.MapFile{Data: []byte("embedded index")},
			"assets/app.js": &fstest.MapFile{Data: []byte("embedded asset")},
		},
	)

	index, err := layered.ReadFile("index.html")
	if err != nil {
		t.Fatalf("read override file: %v", err)
	}
	if got := string(index); got != "override index" {
		t.Fatalf("expected override contents, got %q", got)
	}

	asset, err := layered.ReadFile("assets/app.js")
	if err != nil {
		t.Fatalf("read embedded asset: %v", err)
	}
	if got := string(asset); got != "embedded asset" {
		t.Fatalf("expected embedded asset contents, got %q", got)
	}
}

func TestLayeredFSFallsBackWhenOverrideMissing(t *testing.T) {
	layered := NewLayeredFS(
		fstest.MapFS{
			"assets/override-only.js": &fstest.MapFile{Data: []byte("override asset")},
		},
		fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("embedded index")},
		},
	)

	data, err := layered.ReadFile("index.html")
	if err != nil {
		t.Fatalf("read fallback file: %v", err)
	}
	if got := string(data); got != "embedded index" {
		t.Fatalf("expected embedded fallback contents, got %q", got)
	}
}

func TestBundleManagerActiveChannelDefaultsToStable(t *testing.T) {
	manager := &BundleManager{
		OverrideRoot: filepath.Join(t.TempDir(), ".wailsrel"),
	}

	if got := manager.ActiveChannel(); got != "stable" {
		t.Fatalf("expected default channel stable, got %q", got)
	}

	if err := os.MkdirAll(manager.rootDir(), 0o755); err != nil {
		t.Fatalf("mkdir root dir: %v", err)
	}
	if err := os.WriteFile(manager.channelMarkerPath(), []byte("\n"), 0o644); err != nil {
		t.Fatalf("write empty marker: %v", err)
	}

	if got := manager.ActiveChannel(); got != "stable" {
		t.Fatalf("expected blank marker to default to stable, got %q", got)
	}
}

func TestBundleManagerInstallAndLoadActive(t *testing.T) {
	root := t.TempDir()
	manager := &BundleManager{
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	bundlePath := writeTestBundleArchive(t, root, "stable", BundleManifest{
		Channel:       "stable",
		Version:       "1.2.3",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html":    "<!doctype html>\n<title>stable</title>\n",
		"assets/app.js": "console.log('stable');\n",
	})

	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle archive: %v", err)
	}
	defer bundleFile.Close()

	if err := manager.Install(context.Background(), "stable", bundleFile); err != nil {
		t.Fatalf("install bundle: %v", err)
	}

	filesystem, manifest, err := manager.LoadActive()
	if err != nil {
		t.Fatalf("load active bundle: %v", err)
	}
	if filesystem == nil {
		t.Fatal("expected active filesystem")
	}
	if manifest == nil {
		t.Fatal("expected active manifest")
	}
	if manifest.Channel != "stable" {
		t.Fatalf("expected active channel stable, got %q", manifest.Channel)
	}
	if manifest.Version != "1.2.3" {
		t.Fatalf("expected active version 1.2.3, got %q", manifest.Version)
	}

	index, err := fs.ReadFile(filesystem, "index.html")
	if err != nil {
		t.Fatalf("read active index: %v", err)
	}
	if got := string(index); !strings.Contains(got, "<title>stable</title>") {
		t.Fatalf("expected stable bundle contents, got %q", got)
	}
}

func TestBundleManagerInstallRejectsCompatMismatch(t *testing.T) {
	root := t.TempDir()
	bundlePath := writeTestBundleArchive(t, root, "compat-mismatch", BundleManifest{
		Channel:       "beta",
		Version:       "2.0.0",
		CompatID:      "3",
		CompatVersion: 3,
	}, map[string]string{
		"index.html":    "<!doctype html>\n",
		"assets/app.js": "console.log('bundle');\n",
	})

	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle archive: %v", err)
	}
	defer bundleFile.Close()

	manager := &BundleManager{
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	err = manager.Install(context.Background(), "beta", bundleFile)
	if err == nil || !strings.Contains(err.Error(), "does not match native compat") {
		t.Fatalf("expected compat mismatch error, got %v", err)
	}
}

func TestBundleManagerInstallRejectsChannelMismatch(t *testing.T) {
	root := t.TempDir()
	bundlePath := writeTestBundleArchive(t, root, "channel-mismatch", BundleManifest{
		Channel:       "beta",
		Version:       "2.0.0",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<!doctype html>\n",
	})

	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle archive: %v", err)
	}
	defer bundleFile.Close()

	manager := &BundleManager{
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	err = manager.Install(context.Background(), "stable", bundleFile)
	if err == nil || !strings.Contains(err.Error(), `bundle channel "beta" does not match requested channel "stable"`) {
		t.Fatalf("expected channel mismatch error, got %v", err)
	}
}

func TestBundleManagerListInstalled(t *testing.T) {
	root := t.TempDir()
	manager := &BundleManager{
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	installTestBundle(t, manager, root, "stable", BundleManifest{
		Channel:       "stable",
		Version:       "1.0.0",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>stable</title>\n",
	})
	installTestBundle(t, manager, root, "beta", BundleManifest{
		Channel:       "beta",
		Version:       "1.1.0",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>beta</title>\n",
	})

	installed, err := manager.ListInstalled()
	if err != nil {
		t.Fatalf("list installed bundles: %v", err)
	}
	if len(installed) != 2 {
		t.Fatalf("expected 2 installed bundles, got %d", len(installed))
	}
	if installed[0].Channel != "beta" || installed[0].Version != "1.1.0" {
		t.Fatalf("expected beta bundle first, got %+v", installed[0])
	}
	if installed[1].Channel != "stable" || installed[1].Version != "1.0.0" {
		t.Fatalf("expected stable bundle second, got %+v", installed[1])
	}
}

func TestBundleManagerCleanupRemovesIncompatibleBundles(t *testing.T) {
	root := t.TempDir()
	installManager := &BundleManager{
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	installTestBundle(t, installManager, root, "stable", BundleManifest{
		Channel:       "stable",
		Version:       "1.0.0",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>stable</title>\n",
	})
	installTestBundle(t, installManager, root, "beta", BundleManifest{
		Channel:       "beta",
		Version:       "2.0.0",
		CompatID:      "3",
		CompatVersion: 3,
	}, map[string]string{
		"index.html": "<title>beta</title>\n",
	})

	cleanupManager := &BundleManager{
		NativeCompat: "2",
		OverrideRoot: installManager.OverrideRoot,
	}

	if err := cleanupManager.Cleanup(); err != nil {
		t.Fatalf("cleanup bundles: %v", err)
	}

	installed, err := cleanupManager.ListInstalled()
	if err != nil {
		t.Fatalf("list bundles after cleanup: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("expected 1 installed bundle after cleanup, got %d", len(installed))
	}
	if installed[0].Channel != "stable" {
		t.Fatalf("expected stable bundle to remain, got %+v", installed[0])
	}

	if _, err := os.Stat(cleanupManager.channelDir("beta")); !os.IsNotExist(err) {
		t.Fatalf("expected beta channel directory removed, stat err=%v", err)
	}
}

func TestBundleManagerInstallCodepushRollsBackWhenInstallRecordWriteFails(t *testing.T) {
	root := t.TempDir()
	manager := &BundleManager{
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	initialPath := writeTestBundleArchive(t, root, "codepush-initial", BundleManifest{
		Kind:          BundleKindCodepush,
		Name:          "hotfix-0",
		Version:       "1.0.0",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>hotfix-0</title>\n",
	})
	rewriteBundleChecksum(t, initialPath)
	initialFile, err := os.Open(initialPath)
	if err != nil {
		t.Fatalf("open initial codepush: %v", err)
	}
	defer initialFile.Close()
	if err := manager.InstallCodepush(context.Background(), initialFile); err != nil {
		t.Fatalf("install initial codepush: %v", err)
	}

	if err := os.MkdirAll(manager.installRecordPath(BundleKindCodepush, "hotfix-1"), 0o755); err != nil {
		t.Fatalf("block new install record path: %v", err)
	}

	updatePath := writeTestBundleArchive(t, root, "codepush-update", BundleManifest{
		Kind:          BundleKindCodepush,
		Name:          "hotfix-1",
		Version:       "1.0.1",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>hotfix-1</title>\n",
	})
	rewriteBundleChecksum(t, updatePath)
	updateFile, err := os.Open(updatePath)
	if err != nil {
		t.Fatalf("open updated codepush: %v", err)
	}
	defer updateFile.Close()

	err = manager.InstallCodepush(context.Background(), updateFile)
	if err == nil {
		t.Fatal("expected install record write failure")
	}

	installed, err := manager.LoadInstalledCodepush()
	if err != nil {
		t.Fatalf("load installed codepush after rollback: %v", err)
	}
	if installed == nil || installed.Version != "1.0.0" {
		t.Fatalf("expected original codepush to remain installed, got %+v", installed)
	}
}

func TestBundleManagerInstallCodepushDoesNotSwapBundleWhenContextCancelled(t *testing.T) {
	root := t.TempDir()
	manager := &BundleManager{
		NativeCompat: "2",
		OverrideRoot: filepath.Join(root, ".wailsrel"),
	}

	initialPath := writeTestBundleArchive(t, root, "codepush-initial", BundleManifest{
		Kind:          BundleKindCodepush,
		Name:          "hotfix-0",
		Version:       "1.0.0",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>hotfix-0</title>\n",
	})
	rewriteBundleChecksum(t, initialPath)
	initialFile, err := os.Open(initialPath)
	if err != nil {
		t.Fatalf("open initial codepush: %v", err)
	}
	defer initialFile.Close()
	if err := manager.InstallCodepush(context.Background(), initialFile); err != nil {
		t.Fatalf("install initial codepush: %v", err)
	}

	updatePath := writeTestBundleArchive(t, root, "codepush-update", BundleManifest{
		Kind:          BundleKindCodepush,
		Name:          "hotfix-1",
		Version:       "1.0.1",
		CompatID:      "2",
		CompatVersion: 2,
	}, map[string]string{
		"index.html": "<title>hotfix-1</title>\n",
	})
	rewriteBundleChecksum(t, updatePath)
	updateFile, err := os.Open(updatePath)
	if err != nil {
		t.Fatalf("open updated codepush: %v", err)
	}
	defer updateFile.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = manager.InstallCodepush(ctx, updateFile)
	if err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}

	installed, err := manager.LoadInstalledCodepush()
	if err != nil {
		t.Fatalf("load installed codepush after cancellation: %v", err)
	}
	if installed == nil || installed.Version != "1.0.0" {
		t.Fatalf("expected original codepush to remain installed, got %+v", installed)
	}
}

func installTestBundle(t *testing.T, manager *BundleManager, root, name string, manifest BundleManifest, files map[string]string) {
	t.Helper()

	bundlePath := writeTestBundleArchive(t, root, name, manifest, files)
	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle archive %s: %v", name, err)
	}
	defer bundleFile.Close()

	if err := manager.Install(context.Background(), manifest.Channel, bundleFile); err != nil {
		t.Fatalf("install bundle %s: %v", name, err)
	}
}

func writeTestBundleArchive(t *testing.T, root, name string, manifest BundleManifest, files map[string]string) string {
	t.Helper()

	buildDir := filepath.Join(root, name+"-build")
	for relativePath, contents := range files {
		fullPath := filepath.Join(buildDir, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", relativePath, err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", relativePath, err)
		}
	}

	bundlePath := filepath.Join(root, name+".zip")
	if err := writeBundleArchive(bundlePath, buildDir, manifest); err != nil {
		t.Fatalf("write bundle archive %s: %v", name, err)
	}
	return bundlePath
}
