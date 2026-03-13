package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gabstv/go-bsdiff/pkg/bsdiff"
	"github.com/you/wailsrel/pkg/version"
)

type Options struct {
	OutputDir    string
	CacheDir     string
	FromVersions int
	Source       string
	TagPrefix    string
}

type Generator struct {
	outputDir    string
	cacheDir     string
	fromVersions int
	source       string
	tagPrefix    string
}

type Plan struct {
	OutputDir string    `json:"output_dir"`
	CacheDir  string    `json:"cache_dir"`
	Source    string    `json:"source"`
	Pending   []Pending `json:"pending"`
	Skipped   []Skipped `json:"skipped"`
	Versions  []string  `json:"versions"`
}

type Pending struct {
	FromVersion string `json:"from_version"`
	Artifact    string `json:"artifact"`
	Previous    string `json:"previous"`
	Current     string `json:"current"`
	Patch       string `json:"patch"`
}

type Skipped struct {
	FromVersion string `json:"from_version,omitempty"`
	Artifact    string `json:"artifact"`
	Reason      string `json:"reason"`
}

type Result struct {
	OutputDir string      `json:"output_dir"`
	CacheDir  string      `json:"cache_dir"`
	Source    string      `json:"source"`
	Generated []Generated `json:"generated"`
	Skipped   []Skipped   `json:"skipped"`
}

type Generated struct {
	FromVersion string `json:"from_version"`
	Artifact    string `json:"artifact"`
	Patch       string `json:"patch"`
	Checksum    string `json:"checksum"`
	Size        int64  `json:"size"`
}

type cachedVersion struct {
	name     string
	parsed   version.Version
	cacheDir string
}

type currentArtifact struct {
	relative string
	path     string
	isDir    bool
}

func NewGenerator(opts Options) *Generator {
	return &Generator{
		outputDir:    filepath.Clean(opts.OutputDir),
		cacheDir:     filepath.Clean(opts.CacheDir),
		fromVersions: opts.FromVersions,
		source:       opts.Source,
		tagPrefix:    opts.TagPrefix,
	}
}

func (g *Generator) Plan() (*Plan, error) {
	artifacts, err := discoverCurrentArtifacts(g.outputDir)
	if err != nil {
		return nil, err
	}

	versions, err := discoverCachedVersions(g.cacheDir, g.tagPrefix, g.fromVersions)
	if err != nil {
		return nil, err
	}

	plan := &Plan{
		OutputDir: g.outputDir,
		CacheDir:  g.cacheDir,
		Source:    g.source,
		Pending:   []Pending{},
		Skipped:   []Skipped{},
		Versions:  make([]string, 0, len(versions)),
	}

	for _, cached := range versions {
		plan.Versions = append(plan.Versions, cached.name)
	}

	for _, artifact := range artifacts {
		if artifact.isDir {
			plan.Skipped = append(plan.Skipped, Skipped{
				Artifact: artifact.relative,
				Reason:   "directory artifacts are not supported for delta generation",
			})
			continue
		}

		for _, cached := range versions {
			previousPath := filepath.Join(cached.cacheDir, filepath.FromSlash(artifact.relative))
			info, err := os.Stat(previousPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					plan.Skipped = append(plan.Skipped, Skipped{
						FromVersion: cached.name,
						Artifact:    artifact.relative,
						Reason:      "previous artifact not found",
					})
					continue
				}
				return nil, err
			}
			if info.IsDir() {
				plan.Skipped = append(plan.Skipped, Skipped{
					FromVersion: cached.name,
					Artifact:    artifact.relative,
					Reason:      "directory artifacts are not supported for delta generation",
				})
				continue
			}

			same, err := sameFileContents(previousPath, artifact.path)
			if err != nil {
				return nil, err
			}
			if same {
				plan.Skipped = append(plan.Skipped, Skipped{
					FromVersion: cached.name,
					Artifact:    artifact.relative,
					Reason:      "artifact is unchanged",
				})
				continue
			}

			plan.Pending = append(plan.Pending, Pending{
				FromVersion: cached.name,
				Artifact:    artifact.relative,
				Previous:    previousPath,
				Current:     artifact.path,
				Patch:       filepath.Join(g.outputDir, "delta", cached.name, filepath.FromSlash(artifact.relative)+".bsdiff"),
			})
		}
	}

	if len(plan.Versions) == 0 && (g.source == "github-release" || g.source == "url") {
		return nil, fmt.Errorf("no cached artifacts found in %s; remote fetching for %s is not implemented", g.cacheDir, g.source)
	}

	return plan, nil
}

func (g *Generator) Generate(plan *Plan) (*Result, error) {
	result := &Result{
		OutputDir: g.outputDir,
		CacheDir:  g.cacheDir,
		Source:    g.source,
		Generated: make([]Generated, 0, len(plan.Pending)),
		Skipped:   append([]Skipped(nil), plan.Skipped...),
	}

	for _, pending := range plan.Pending {
		if err := os.MkdirAll(filepath.Dir(pending.Patch), 0o755); err != nil {
			return nil, err
		}
		if err := bsdiff.File(pending.Previous, pending.Current, pending.Patch); err != nil {
			return nil, fmt.Errorf("generate patch for %s from %s: %w", pending.Artifact, pending.FromVersion, err)
		}

		checksum, size, err := fileDigest(pending.Patch)
		if err != nil {
			return nil, err
		}

		result.Generated = append(result.Generated, Generated{
			FromVersion: pending.FromVersion,
			Artifact:    pending.Artifact,
			Patch:       pending.Patch,
			Checksum:    "sha256:" + checksum,
			Size:        size,
		})
	}

	return result, nil
}

func discoverCurrentArtifacts(outputDir string) ([]currentArtifact, error) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, err
	}

	artifacts := make([]currentArtifact, 0)
	for _, osEntry := range entries {
		if !osEntry.IsDir() || osEntry.Name() == "delta" {
			continue
		}

		osDir := filepath.Join(outputDir, osEntry.Name())
		archEntries, err := os.ReadDir(osDir)
		if err != nil {
			return nil, err
		}

		for _, archEntry := range archEntries {
			if !archEntry.IsDir() {
				continue
			}

			archDir := filepath.Join(osDir, archEntry.Name())
			files, err := os.ReadDir(archDir)
			if err != nil {
				return nil, err
			}

			for _, artifactEntry := range files {
				name := artifactEntry.Name()
				if strings.HasSuffix(name, ".sha256") {
					continue
				}

				artifacts = append(artifacts, currentArtifact{
					relative: filepath.ToSlash(filepath.Join(osEntry.Name(), archEntry.Name(), name)),
					path:     filepath.Join(archDir, name),
					isDir:    artifactEntry.IsDir(),
				})
			}
		}
	}

	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].relative < artifacts[j].relative
	})

	return artifacts, nil
}

func discoverCachedVersions(cacheRoot, tagPrefix string, limit int) ([]cachedVersion, error) {
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	versions := make([]cachedVersion, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		raw := name
		if tagPrefix != "" && strings.HasPrefix(raw, tagPrefix) {
			raw = strings.TrimPrefix(raw, tagPrefix)
		}

		parsed, err := version.Parse(raw)
		if err != nil {
			continue
		}

		versions = append(versions, cachedVersion{
			name:     name,
			parsed:   parsed,
			cacheDir: filepath.Join(cacheRoot, name),
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].parsed.Compare(versions[j].parsed) > 0
	})

	if limit > 0 && len(versions) > limit {
		versions = versions[:limit]
	}

	return versions, nil
}

func sameFileContents(a, b string) (bool, error) {
	infoA, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	infoB, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	if infoA.Size() != infoB.Size() {
		return false, nil
	}

	checksumA, _, err := fileDigest(a)
	if err != nil {
		return false, err
	}
	checksumB, _, err := fileDigest(b)
	if err != nil {
		return false, err
	}

	return checksumA == checksumB, nil
}

func fileDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}

	return hex.EncodeToString(hash.Sum(nil)), size, nil
}
