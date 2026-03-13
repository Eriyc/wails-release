package delta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBSDiffApplyRoundTrip(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.bin")
	newPath := filepath.Join(root, "new.bin")
	patchPath := filepath.Join(root, "update.patch")
	appliedPath := filepath.Join(root, "applied.bin")

	oldBinary := []byte("old release binary data")
	newBinary := []byte("new release binary data with appended bytes")
	if err := os.WriteFile(oldPath, oldBinary, 0o755); err != nil {
		t.Fatalf("write old binary: %v", err)
	}
	if err := os.WriteFile(newPath, newBinary, 0o755); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	info, err := Generate(oldPath, newPath, patchPath)
	if err != nil {
		t.Fatalf("generate patch: %v", err)
	}

	if _, err := Apply(oldPath, patchPath, appliedPath); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	appliedBytes, err := os.ReadFile(appliedPath)
	if err != nil {
		t.Fatalf("read applied binary: %v", err)
	}
	if string(appliedBytes) != string(newBinary) {
		t.Fatalf("expected applied binary %q, got %q", string(newBinary), string(appliedBytes))
	}

	if appliedChecksum, _, err := fileDigest(appliedPath); err != nil {
		t.Fatalf("digest applied binary: %v", err)
	} else if appliedChecksum != info.ToChecksum {
		t.Fatalf("expected applied checksum %q, got %q", info.ToChecksum, appliedChecksum)
	}
}
