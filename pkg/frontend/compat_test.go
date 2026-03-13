package frontend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeCompatHashIsDeterministic(t *testing.T) {
	root := t.TempDir()
	bindingsDir := filepath.Join(root, "bindings")
	if err := os.MkdirAll(filepath.Join(bindingsDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir bindings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bindingsDir, "api.ts"), []byte("export const value = 1;\n"), 0o644); err != nil {
		t.Fatalf("write api.ts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bindingsDir, "nested", "types.ts"), []byte("export type ID = string;\n"), 0o644); err != nil {
		t.Fatalf("write types.ts: %v", err)
	}

	first, err := ComputeCompatHash(bindingsDir)
	if err != nil {
		t.Fatalf("compute compat hash: %v", err)
	}
	second, err := ComputeCompatHash(bindingsDir)
	if err != nil {
		t.Fatalf("compute compat hash second: %v", err)
	}
	if first != second {
		t.Fatalf("expected deterministic hash, got %q != %q", first, second)
	}
}

func TestCheckCompatSnapshotWarnsWhenHashChangesWithoutVersionBump(t *testing.T) {
	root := t.TempDir()
	snapshotPath := filepath.Join(root, "compat.json")
	if err := WriteCompatSnapshot(snapshotPath, CompatSnapshot{
		CompatVersion: 1,
		BindingsHash:  "old-hash",
	}); err != nil {
		t.Fatalf("write compat snapshot: %v", err)
	}

	warning, err := CheckCompatSnapshot(snapshotPath, 1, "new-hash")
	if err != nil {
		t.Fatalf("check compat snapshot: %v", err)
	}
	if warning == "" {
		t.Fatal("expected compat warning")
	}

	warning, err = CheckCompatSnapshot(snapshotPath, 2, "new-hash")
	if err != nil {
		t.Fatalf("check compat snapshot with bumped version: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning after version bump, got %q", warning)
	}
}

func TestComputeCompatHashChangesWhenBindingsChange(t *testing.T) {
	root := t.TempDir()
	bindingsDir := filepath.Join(root, "bindings")
	if err := os.MkdirAll(bindingsDir, 0o755); err != nil {
		t.Fatalf("mkdir bindings: %v", err)
	}
	path := filepath.Join(bindingsDir, "api.ts")
	if err := os.WriteFile(path, []byte("export const value = 1;\n"), 0o644); err != nil {
		t.Fatalf("write initial bindings: %v", err)
	}

	first, err := ComputeCompatHash(bindingsDir)
	if err != nil {
		t.Fatalf("compute initial hash: %v", err)
	}

	if err := os.WriteFile(path, []byte("export const value = 2;\n"), 0o644); err != nil {
		t.Fatalf("write updated bindings: %v", err)
	}

	second, err := ComputeCompatHash(bindingsDir)
	if err != nil {
		t.Fatalf("compute updated hash: %v", err)
	}
	if first == second {
		t.Fatalf("expected bindings hash to change after content update, got %q", first)
	}
}

func TestWriteAndLoadCompatSnapshotRoundTrip(t *testing.T) {
	root := t.TempDir()
	snapshotPath := filepath.Join(root, "nested", "compat.json")

	if err := WriteCompatSnapshot(snapshotPath, CompatSnapshot{
		CompatVersion: 7,
		BindingsHash:  "bindings-hash",
	}); err != nil {
		t.Fatalf("write compat snapshot: %v", err)
	}

	snapshot, err := LoadCompatSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("load compat snapshot: %v", err)
	}
	if snapshot.CompatVersion != 7 {
		t.Fatalf("expected compat version 7, got %d", snapshot.CompatVersion)
	}
	if snapshot.BindingsHash != "bindings-hash" {
		t.Fatalf("expected bindings hash %q, got %q", "bindings-hash", snapshot.BindingsHash)
	}
	if snapshot.UpdatedAt.IsZero() {
		t.Fatal("expected snapshot updated_at to be set")
	}

	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read compat snapshot: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("expected compat snapshot to end with newline, got %q", string(data))
	}
}

func TestCheckCompatSnapshotExpectedBehaviors(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name          string
		setup         func(t *testing.T, path string)
		compatVersion int
		bindingsHash  string
		wantWarning   string
		wantErrSubstr string
	}{
		{
			name:          "missing snapshot is allowed",
			setup:         func(t *testing.T, path string) {},
			compatVersion: 1,
			bindingsHash:  "new-hash",
		},
		{
			name: "empty stored hash does not warn",
			setup: func(t *testing.T, path string) {
				if err := WriteCompatSnapshot(path, CompatSnapshot{
					CompatVersion: 1,
					BindingsHash:  "",
				}); err != nil {
					t.Fatalf("write compat snapshot: %v", err)
				}
			},
			compatVersion: 1,
			bindingsHash:  "new-hash",
		},
		{
			name: "matching hash does not warn",
			setup: func(t *testing.T, path string) {
				if err := WriteCompatSnapshot(path, CompatSnapshot{
					CompatVersion: 1,
					BindingsHash:  "same-hash",
				}); err != nil {
					t.Fatalf("write compat snapshot: %v", err)
				}
			},
			compatVersion: 1,
			bindingsHash:  "same-hash",
		},
		{
			name: "changed hash without version bump warns",
			setup: func(t *testing.T, path string) {
				if err := WriteCompatSnapshot(path, CompatSnapshot{
					CompatVersion: 2,
					BindingsHash:  "old-hash",
				}); err != nil {
					t.Fatalf("write compat snapshot: %v", err)
				}
			},
			compatVersion: 2,
			bindingsHash:  "new-hash",
			wantWarning:   "frontend.compat_version is still 2",
		},
		{
			name: "changed hash with version bump does not warn",
			setup: func(t *testing.T, path string) {
				if err := WriteCompatSnapshot(path, CompatSnapshot{
					CompatVersion: 2,
					BindingsHash:  "old-hash",
				}); err != nil {
					t.Fatalf("write compat snapshot: %v", err)
				}
			},
			compatVersion: 3,
			bindingsHash:  "new-hash",
		},
		{
			name: "invalid snapshot returns error",
			setup: func(t *testing.T, path string) {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir snapshot dir: %v", err)
				}
				if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
					t.Fatalf("write invalid snapshot: %v", err)
				}
			},
			compatVersion: 1,
			bindingsHash:  "new-hash",
			wantErrSubstr: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshotPath := filepath.Join(root, strings.ReplaceAll(tt.name, " ", "_"), "compat.json")
			tt.setup(t, snapshotPath)

			warning, err := CheckCompatSnapshot(snapshotPath, tt.compatVersion, tt.bindingsHash)
			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErrSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("check compat snapshot: %v", err)
			}
			if tt.wantWarning == "" && warning != "" {
				t.Fatalf("expected no warning, got %q", warning)
			}
			if tt.wantWarning != "" && !strings.Contains(warning, tt.wantWarning) {
				t.Fatalf("expected warning containing %q, got %q", tt.wantWarning, warning)
			}
		})
	}
}
