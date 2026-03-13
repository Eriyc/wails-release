package delta

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gabstv/go-bsdiff/pkg/bsdiff"
	"github.com/gabstv/go-bsdiff/pkg/bspatch"
)

type ArtifactKind string

const (
	ArtifactFile      ArtifactKind = "file"
	ArtifactDirectory ArtifactKind = "directory"
)

type PatchInfo struct {
	ArtifactKind   ArtifactKind `json:"artifact_kind"`
	FromChecksum   string       `json:"from_checksum"`
	ToChecksum     string       `json:"to_checksum"`
	PatchChecksum  string       `json:"patch_checksum"`
	FromSize       int64        `json:"from_size"`
	ToSize         int64        `json:"to_size"`
	PatchSize      int64        `json:"patch_size"`
	SavingsBytes   int64        `json:"savings_bytes"`
	SavingsPercent float64      `json:"savings_percent"`
}

type ApplyResult struct {
	ArtifactKind ArtifactKind `json:"artifact_kind"`
	Checksum     string       `json:"checksum"`
	Size         int64        `json:"size"`
}

func Generate(oldPath, newPath, patchPath string) (*PatchInfo, error) {
	oldPrepared, oldCleanup, oldKind, err := prepareComparableArtifact(oldPath)
	if err != nil {
		return nil, err
	}
	defer oldCleanup()

	newPrepared, newCleanup, newKind, err := prepareComparableArtifact(newPath)
	if err != nil {
		return nil, err
	}
	defer newCleanup()

	if oldKind != newKind {
		return nil, fmt.Errorf("artifact kind mismatch: %s != %s", oldKind, newKind)
	}
	if err := os.MkdirAll(filepath.Dir(patchPath), 0o755); err != nil {
		return nil, err
	}
	if err := bsdiff.File(oldPrepared, newPrepared, patchPath); err != nil {
		return nil, err
	}

	_, fromChecksum, fromSize, err := artifactSummary(oldPath)
	if err != nil {
		return nil, err
	}
	_, toChecksum, toSize, err := artifactSummary(newPath)
	if err != nil {
		return nil, err
	}
	patchChecksum, patchSize, err := fileDigest(patchPath)
	if err != nil {
		return nil, err
	}

	info := &PatchInfo{
		ArtifactKind:  oldKind,
		FromChecksum:  fromChecksum,
		ToChecksum:    toChecksum,
		PatchChecksum: patchChecksum,
		FromSize:      fromSize,
		ToSize:        toSize,
		PatchSize:     patchSize,
		SavingsBytes:  toSize - patchSize,
	}
	if toSize > 0 {
		info.SavingsPercent = (float64(info.SavingsBytes) / float64(toSize)) * 100
	}

	return info, nil
}

func Apply(oldPath, patchPath, outputPath string) (*ApplyResult, error) {
	preparedOld, cleanup, kind, err := prepareComparableArtifact(oldPath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	switch kind {
	case ArtifactFile:
		if err := applyFilePatch(oldPath, patchPath, outputPath); err != nil {
			return nil, err
		}
	case ArtifactDirectory:
		newArchive, archiveCleanup, err := createTemporaryFile("wailsrel-delta-new-*.tar")
		if err != nil {
			return nil, err
		}
		defer archiveCleanup()

		if err := bspatch.File(preparedOld, newArchive, patchPath); err != nil {
			return nil, err
		}
		if err := extractTarArchive(newArchive, outputPath); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported artifact kind %s", kind)
	}

	_, checksum, size, err := artifactSummary(outputPath)
	if err != nil {
		return nil, err
	}

	return &ApplyResult{
		ArtifactKind: kind,
		Checksum:     checksum,
		Size:         size,
	}, nil
}

func detectArtifactKind(path string) (ArtifactKind, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return ArtifactDirectory, nil
	}
	return ArtifactFile, nil
}

func artifactSummary(path string) (ArtifactKind, string, int64, error) {
	kind, err := detectArtifactKind(path)
	if err != nil {
		return "", "", 0, err
	}
	if kind == ArtifactFile {
		checksum, size, err := fileDigest(path)
		return kind, checksum, size, err
	}

	checksum, size, err := digestDirectory(path)
	return kind, checksum, size, err
}

func prepareComparableArtifact(path string) (string, func(), ArtifactKind, error) {
	kind, err := detectArtifactKind(path)
	if err != nil {
		return "", nil, "", err
	}
	if kind == ArtifactFile {
		return path, func() {}, kind, nil
	}

	archivePath, cleanup, err := createTemporaryFile("wailsrel-delta-archive-*.tar")
	if err != nil {
		return "", nil, "", err
	}
	if err := archiveDirectoryToTar(path, archivePath); err != nil {
		cleanup()
		return "", nil, "", err
	}

	return archivePath, cleanup, kind, nil
}

func createTemporaryFile(pattern string) (string, func(), error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", nil, err
	}
	cleanup := func() {
		_ = os.Remove(path)
	}
	return path, cleanup, nil
}

func applyFilePatch(oldPath, patchPath, outputPath string) error {
	oldBytes, err := os.ReadFile(oldPath)
	if err != nil {
		return err
	}
	patchBytes, err := os.ReadFile(patchPath)
	if err != nil {
		return err
	}
	newBytes, err := bspatch.Bytes(oldBytes, patchBytes)
	if err != nil {
		return err
	}
	info, err := os.Stat(oldPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, newBytes, info.Mode().Perm())
}
