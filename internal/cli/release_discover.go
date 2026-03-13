package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/Eriyc/wailsrel/pkg/delta"
)

func discoverReleaseArtifacts(outputDir string) ([]build.Artifact, error) {
	if artifacts, err := readArtifactMetadata(outputDir); err == nil {
		return artifacts, nil
	}

	entries := make([]build.Artifact, 0)
	seen := make(map[string]struct{})

	err := filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == outputDir {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".sha256") || strings.HasPrefix(filepath.ToSlash(path), filepath.ToSlash(filepath.Join(outputDir, "delta"))) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(outputDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 3 {
			return nil
		}
		logicalPath := filepath.ToSlash(rel)
		rootKey := filepath.ToSlash(filepath.Join(parts[0], parts[1], parts[2]))
		if _, ok := seen[rootKey]; ok {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() && logicalPath != rootKey {
			return nil
		}

		checksum, err := build.ComputeChecksum(path)
		if err != nil {
			return err
		}
		size, err := releasePathSize(path)
		if err != nil {
			return err
		}

		entries = append(entries, build.Artifact{
			Path:              logicalPath,
			OS:                parts[0],
			Arch:              parts[1],
			Format:            inferArtifactFormat(path, info.IsDir()),
			Checksum:          "sha256:" + checksum,
			Size:              size,
			Metadata:          map[string]string{},
			IncludeInManifest: true,
			EnableDelta:       true,
		})
		seen[rootKey] = struct{}{}
		if info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	if len(entries) == 0 {
		return nil, fmt.Errorf("no release artifacts found in %s; run build first or disable --dry-run", outputDir)
	}
	return entries, nil
}

func discoverReleaseDelta(outputDir string) (*delta.Result, error) {
	manifestPath := filepath.Join(outputDir, "delta", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var manifest delta.PatchManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if err := delta.ValidateManifest(&manifest); err != nil {
		return nil, err
	}

	result := &delta.Result{
		OutputDir:    outputDir,
		ManifestPath: manifestPath,
		Generated:    make([]delta.Generated, 0, len(manifest.Patches)),
	}
	for _, patch := range manifest.Patches {
		result.Generated = append(result.Generated, delta.Generated{
			FromVersion:    patch.FromVersion,
			Artifact:       patch.Artifact,
			ArtifactKind:   patch.ArtifactKind,
			Patch:          filepath.Join(outputDir, filepath.FromSlash(patch.Patch)),
			Checksum:       "sha256:" + patch.PatchSHA256,
			FromChecksum:   "sha256:" + patch.FromSHA256,
			ToChecksum:     "sha256:" + patch.ToSHA256,
			FromSize:       patch.FromSize,
			ToSize:         patch.ToSize,
			Size:           patch.PatchSize,
			SavingsBytes:   patch.SavingsBytes,
			SavingsPercent: patch.SavingsPercent,
		})
	}
	return result, nil
}

func inferArtifactFormat(path string, isDir bool) string {
	if isDir {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".app":
			return "app"
		default:
			return "directory"
		}
	}
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".dmg"):
		return "dmg"
	case strings.HasSuffix(lower, ".appimage"):
		return "appimage"
	case strings.HasSuffix(lower, ".deb"):
		return "deb"
	case strings.HasSuffix(lower, ".rpm"):
		return "rpm"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".exe"):
		return "exe"
	default:
		return strings.TrimPrefix(filepath.Ext(path), ".")
	}
}

func releasePathSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}

	var total int64
	err = filepath.Walk(path, func(current string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}
