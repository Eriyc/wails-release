package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eriyc/wailsrel/pkg/config"
	"github.com/Eriyc/wailsrel/pkg/frontend"
)

func TestCheckFrontendCompatSkipsMissingBindingsWithoutSnapshot(t *testing.T) {
	repo := t.TempDir()
	result, err := checkFrontendCompat(repo, &config.Config{
		Frontend: config.FrontendConfig{
			Enabled:         true,
			CompatVersion:   1,
			CompatAutoCheck: true,
			BindingsDir:     "frontend/bindings",
		},
	})
	if err != nil {
		t.Fatalf("check frontend compat: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result when bindings are absent and no snapshot exists, got %+v", result)
	}
}

func TestCheckFrontendCompatWarnsWhenBindingsChangeWithoutCompatVersionBump(t *testing.T) {
	repo := t.TempDir()
	cfg := testFrontendCompatConfig()

	bindingsDir := filepath.Join(repo, cfg.Frontend.BindingsDir)
	if err := os.MkdirAll(bindingsDir, 0o755); err != nil {
		t.Fatalf("mkdir bindings dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bindingsDir, "backend.ts"), []byte("export const api = 1;\n"), 0o644); err != nil {
		t.Fatalf("write bindings: %v", err)
	}

	if err := frontend.WriteCompatSnapshot(frontendCompatSnapshotPath(repo), frontend.CompatSnapshot{
		CompatVersion: cfg.Frontend.CompatVersion,
		BindingsHash:  "old-hash",
	}); err != nil {
		t.Fatalf("write compat snapshot: %v", err)
	}

	result, err := checkFrontendCompat(repo, cfg)
	if err != nil {
		t.Fatalf("check frontend compat: %v", err)
	}
	if result == nil {
		t.Fatal("expected compat result")
	}
	if result.BindingsHash == "" {
		t.Fatal("expected bindings hash to be populated")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected one warning, got %+v", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "frontend bindings changed but frontend.compat_version is still 1") {
		t.Fatalf("expected compat version warning, got %q", result.Warnings[0])
	}
}

func TestCheckFrontendCompatWarnsWhenBindingsAreMissingButSnapshotExists(t *testing.T) {
	repo := t.TempDir()
	cfg := testFrontendCompatConfig()

	if err := frontend.WriteCompatSnapshot(frontendCompatSnapshotPath(repo), frontend.CompatSnapshot{
		CompatVersion: cfg.Frontend.CompatVersion,
		BindingsHash:  "old-hash",
	}); err != nil {
		t.Fatalf("write compat snapshot: %v", err)
	}

	result, err := checkFrontendCompat(repo, cfg)
	if err != nil {
		t.Fatalf("check frontend compat: %v", err)
	}
	if result == nil {
		t.Fatal("expected compat result")
	}
	if result.BindingsHash != "" {
		t.Fatalf("expected empty bindings hash, got %q", result.BindingsHash)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected one warning, got %+v", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "frontend compat check skipped:") {
		t.Fatalf("expected skipped warning, got %q", result.Warnings[0])
	}
	if !strings.Contains(result.Warnings[0], filepath.Join(repo, cfg.Frontend.BindingsDir)) {
		t.Fatalf("expected missing bindings path in warning, got %q", result.Warnings[0])
	}
}

func TestPersistFrontendCompatWritesSnapshot(t *testing.T) {
	repo := t.TempDir()
	cfg := testFrontendCompatConfig()
	result := &frontendCompatResult{BindingsHash: "computed-hash"}

	if err := persistFrontendCompat(repo, cfg, result); err != nil {
		t.Fatalf("persist frontend compat: %v", err)
	}

	snapshot, err := frontend.LoadCompatSnapshot(frontendCompatSnapshotPath(repo))
	if err != nil {
		t.Fatalf("load compat snapshot: %v", err)
	}
	if snapshot.CompatVersion != cfg.Frontend.CompatVersion {
		t.Fatalf("expected compat version %d, got %d", cfg.Frontend.CompatVersion, snapshot.CompatVersion)
	}
	if snapshot.BindingsHash != result.BindingsHash {
		t.Fatalf("expected bindings hash %q, got %q", result.BindingsHash, snapshot.BindingsHash)
	}
	if snapshot.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be set")
	}
}

func testFrontendCompatConfig() *config.Config {
	return &config.Config{
		Frontend: config.FrontendConfig{
			Enabled:         true,
			CompatVersion:   1,
			CompatAutoCheck: true,
			BindingsDir:     "frontend/bindings",
		},
	}
}
