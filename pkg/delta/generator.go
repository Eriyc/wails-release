package delta

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/you/wailsrel/pkg/version"
)

const manifestFileName = "manifest.json"

type Options struct {
	OutputDir        string
	CacheDir         string
	FromVersions     int
	Source           string
	TagPrefix        string
	Repository       string
	ManifestURL      string
	AuthToken        string
	GitHubAPIBaseURL string
	HTTPClient       *http.Client
}

type Generator struct {
	outputDir        string
	cacheDir         string
	fromVersions     int
	source           string
	tagPrefix        string
	repository       string
	manifestURL      string
	authToken        string
	githubAPIBaseURL string
	httpClient       *http.Client
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
	FromVersion  string       `json:"from_version"`
	Artifact     string       `json:"artifact"`
	ArtifactKind ArtifactKind `json:"artifact_kind"`
	Previous     string       `json:"previous"`
	Current      string       `json:"current"`
	Patch        string       `json:"patch"`
}

type Skipped struct {
	FromVersion string `json:"from_version,omitempty"`
	Artifact    string `json:"artifact"`
	Reason      string `json:"reason"`
}

type Result struct {
	OutputDir    string      `json:"output_dir"`
	CacheDir     string      `json:"cache_dir"`
	Source       string      `json:"source"`
	ManifestPath string      `json:"manifest_path"`
	Generated    []Generated `json:"generated"`
	Skipped      []Skipped   `json:"skipped"`
}

type Generated struct {
	FromVersion    string       `json:"from_version"`
	Artifact       string       `json:"artifact"`
	ArtifactKind   ArtifactKind `json:"artifact_kind"`
	Patch          string       `json:"patch"`
	Checksum       string       `json:"checksum"`
	FromChecksum   string       `json:"from_checksum"`
	ToChecksum     string       `json:"to_checksum"`
	FromSize       int64        `json:"from_size"`
	ToSize         int64        `json:"to_size"`
	Size           int64        `json:"size"`
	SavingsBytes   int64        `json:"savings_bytes"`
	SavingsPercent float64      `json:"savings_percent"`
}

type cachedVersion struct {
	name     string
	parsed   version.Version
	cacheDir string
}

type currentArtifact struct {
	relative string
	path     string
	kind     ArtifactKind
}

func NewGenerator(opts Options) *Generator {
	baseURL := strings.TrimSpace(opts.GitHubAPIBaseURL)
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &Generator{
		outputDir:        filepath.Clean(opts.OutputDir),
		cacheDir:         filepath.Clean(opts.CacheDir),
		fromVersions:     opts.FromVersions,
		source:           opts.Source,
		tagPrefix:        opts.TagPrefix,
		repository:       strings.TrimSpace(opts.Repository),
		manifestURL:      strings.TrimSpace(opts.ManifestURL),
		authToken:        strings.TrimSpace(opts.AuthToken),
		githubAPIBaseURL: strings.TrimRight(baseURL, "/"),
		httpClient:       client,
	}
}

func (g *Generator) Plan() (*Plan, error) {
	return g.PlanContext(context.Background())
}

func (g *Generator) PlanContext(ctx context.Context) (*Plan, error) {
	artifacts, err := discoverCurrentArtifacts(g.outputDir)
	if err != nil {
		return nil, err
	}

	if err := g.prepareCache(ctx, artifacts); err != nil {
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
		for _, cached := range versions {
			previousPath := filepath.Join(cached.cacheDir, filepath.FromSlash(artifact.relative))
			previousKind, err := detectArtifactKind(previousPath)
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
			if previousKind != artifact.kind {
				plan.Skipped = append(plan.Skipped, Skipped{
					FromVersion: cached.name,
					Artifact:    artifact.relative,
					Reason:      fmt.Sprintf("artifact kind mismatch: current=%s previous=%s", artifact.kind, previousKind),
				})
				continue
			}

			same, err := sameArtifactContents(previousPath, artifact.path)
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
				FromVersion:  cached.name,
				Artifact:     artifact.relative,
				ArtifactKind: artifact.kind,
				Previous:     previousPath,
				Current:      artifact.path,
				Patch:        filepath.Join(g.outputDir, "delta", cached.name, filepath.FromSlash(artifact.relative)+".patch"),
			})
		}
	}

	if len(plan.Versions) == 0 && strings.TrimSpace(g.source) == "github-release" {
		return nil, fmt.Errorf("no cached artifacts found in %s after fetching GitHub releases for %s", g.cacheDir, g.repository)
	}
	if len(plan.Versions) == 0 && strings.TrimSpace(g.source) == "url" {
		return nil, fmt.Errorf("no cached artifacts found in %s after fetching release manifest %s", g.cacheDir, g.manifestURL)
	}

	return plan, nil
}

func (g *Generator) Generate(plan *Plan) (*Result, error) {
	return g.GenerateContext(context.Background(), plan)
}

func (g *Generator) GenerateContext(_ context.Context, plan *Plan) (*Result, error) {
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

		info, err := Generate(pending.Previous, pending.Current, pending.Patch)
		if err != nil {
			return nil, fmt.Errorf("generate patch for %s from %s: %w", pending.Artifact, pending.FromVersion, err)
		}

		result.Generated = append(result.Generated, Generated{
			FromVersion:    pending.FromVersion,
			Artifact:       pending.Artifact,
			ArtifactKind:   info.ArtifactKind,
			Patch:          pending.Patch,
			Checksum:       "sha256:" + info.PatchChecksum,
			FromChecksum:   "sha256:" + info.FromChecksum,
			ToChecksum:     "sha256:" + info.ToChecksum,
			FromSize:       info.FromSize,
			ToSize:         info.ToSize,
			Size:           info.PatchSize,
			SavingsBytes:   info.SavingsBytes,
			SavingsPercent: info.SavingsPercent,
		})
	}

	manifest := BuildManifest(g.outputDir, result.Generated)
	manifestPath := filepath.Join(g.outputDir, "delta", manifestFileName)
	if err := WriteManifest(manifest, manifestPath); err != nil {
		return nil, err
	}
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	result.ManifestPath = manifestPath

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

				kind := ArtifactFile
				if artifactEntry.IsDir() {
					kind = ArtifactDirectory
				}

				artifacts = append(artifacts, currentArtifact{
					relative: filepath.ToSlash(filepath.Join(osEntry.Name(), archEntry.Name(), name)),
					path:     filepath.Join(archDir, name),
					kind:     kind,
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
		if errors.Is(err, os.ErrNotExist) {
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

func sameArtifactContents(a, b string) (bool, error) {
	kindA, checksumA, _, err := artifactSummary(a)
	if err != nil {
		return false, err
	}
	kindB, checksumB, _, err := artifactSummary(b)
	if err != nil {
		return false, err
	}

	return kindA == kindB && checksumA == checksumB, nil
}
