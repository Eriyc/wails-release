package delta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Eriyc/wailsrel/pkg/version"
)

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Draft   bool          `json:"draft"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type urlReleaseManifest struct {
	Release struct {
		Tag string `json:"tag"`
	} `json:"release"`
	Artifacts []urlReleaseArtifact `json:"artifacts"`
}

type urlReleaseArtifact struct {
	Path      string `json:"path"`
	AssetName string `json:"asset_name"`
	URL       string `json:"url"`
	Checksum  string `json:"checksum"`
}

func (g *Generator) prepareCache(ctx context.Context, artifacts []currentArtifact) error {
	switch strings.TrimSpace(g.source) {
	case "", "local-cache":
		return nil
	case "github-release":
		return g.prepareGitHubCache(ctx, artifacts)
	case "url":
		return g.prepareURLCache(ctx, artifacts)
	default:
		return fmt.Errorf("unsupported delta source %q", g.source)
	}
}

func (g *Generator) prepareGitHubCache(ctx context.Context, artifacts []currentArtifact) error {
	if strings.TrimSpace(g.repository) == "" {
		return fmt.Errorf("delta.old_artifacts.repository must be set or resolvable from git when source=github-release")
	}

	releases, err := g.listGitHubReleases(ctx)
	if err != nil {
		return err
	}

	downloaded := 0
	for _, release := range releases {
		if release.Draft {
			continue
		}
		raw := strings.TrimSpace(release.TagName)
		if raw == "" {
			continue
		}
		versionText := raw
		if g.tagPrefix != "" && strings.HasPrefix(versionText, g.tagPrefix) {
			versionText = strings.TrimPrefix(versionText, g.tagPrefix)
		}
		if _, err := version.Parse(versionText); err != nil {
			continue
		}

		versionDir := filepath.Join(g.cacheDir, release.TagName)
		if err := os.MkdirAll(versionDir, 0o755); err != nil {
			return err
		}

		for _, artifact := range artifacts {
			targetPath := filepath.Join(versionDir, filepath.FromSlash(artifact.relative))
			if _, err := os.Stat(targetPath); err == nil {
				continue
			}
			asset, ok := findGitHubAsset(release.Assets, artifact)
			if !ok {
				continue
			}
			if err := g.downloadAssetToCache(ctx, asset, targetPath, artifact.kind); err != nil {
				return fmt.Errorf("cache %s from %s: %w", artifact.relative, release.TagName, err)
			}
		}

		downloaded++
		if g.fromVersions > 0 && downloaded >= g.fromVersions {
			break
		}
	}

	return nil
}

func (g *Generator) prepareURLCache(ctx context.Context, artifacts []currentArtifact) error {
	if g.manifestURL == "" {
		return fmt.Errorf("delta.old_artifacts.manifest_url must be set when source=url")
	}

	manifest, err := g.fetchURLManifest(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(manifest.Release.Tag) == "" {
		return fmt.Errorf("release manifest %s is missing release.tag", g.manifestURL)
	}

	versionDir := filepath.Join(g.cacheDir, manifest.Release.Tag)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return err
	}

	entries := make(map[string]urlReleaseArtifact, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		entries[filepath.ToSlash(strings.TrimSpace(artifact.Path))] = artifact
	}

	for _, artifact := range artifacts {
		targetPath := filepath.Join(versionDir, filepath.FromSlash(artifact.relative))
		if _, err := os.Stat(targetPath); err == nil {
			continue
		}
		entry, ok := entries[artifact.relative]
		if !ok {
			continue
		}
		if err := g.downloadURLArtifactToCache(ctx, entry, targetPath, artifact.kind); err != nil {
			return fmt.Errorf("cache %s from %s: %w", artifact.relative, manifest.Release.Tag, err)
		}
	}

	return nil
}

func (g *Generator) fetchURLManifest(ctx context.Context) (*urlReleaseManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.manifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "wailsrel")
	if g.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+g.authToken)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("release manifest %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var manifest urlReleaseManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (g *Generator) listGitHubReleases(ctx context.Context) ([]githubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=%d", g.githubAPIBaseURL, g.repository, max(g.fromVersions*2, 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wailsrel")
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("github releases API %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func findGitHubAsset(assets []githubAsset, artifact currentArtifact) (githubAsset, bool) {
	candidates := assetCandidates(artifact)
	for _, candidate := range candidates {
		for _, asset := range assets {
			if asset.Name == candidate {
				return asset, true
			}
		}
	}
	return githubAsset{}, false
}

func assetCandidates(artifact currentArtifact) []string {
	base := filepath.Base(artifact.relative)
	candidates := []string{
		filepath.ToSlash(artifact.relative),
		base,
	}
	if artifact.kind == ArtifactDirectory {
		candidates = append(candidates,
			base+".tar",
			base+".tar.gz",
			base+".tgz",
			base+".zip",
		)
	}
	return uniqueStrings(candidates)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (g *Generator) downloadAssetToCache(ctx context.Context, asset githubAsset, targetPath string, kind ArtifactKind) error {
	tempPath, cleanup, err := createTemporaryFile("wailsrel-github-asset-*")
	if err != nil {
		return err
	}
	defer cleanup()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "wailsrel")
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download %s: %s", asset.Name, strings.TrimSpace(string(body)))
	}

	out, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	if err := materializeDownloadedAsset(tempPath, asset.Name, targetPath, kind); err != nil {
		return err
	}
	return nil
}

func (g *Generator) downloadURLArtifactToCache(ctx context.Context, artifact urlReleaseArtifact, targetPath string, kind ArtifactKind) error {
	tempPath, cleanup, err := createTemporaryFile("wailsrel-url-asset-*")
	if err != nil {
		return err
	}
	defer cleanup()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "wailsrel")
	if g.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+g.authToken)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download %s: %s", artifact.AssetName, strings.TrimSpace(string(body)))
	}

	out, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	if err := materializeDownloadedAsset(tempPath, artifact.AssetName, targetPath, kind); err != nil {
		return err
	}
	return nil
}

func materializeDownloadedAsset(downloadPath, assetName, targetPath string, kind ArtifactKind) error {
	switch kind {
	case ArtifactFile:
		if isGzipTarAsset(assetName) || strings.HasSuffix(strings.ToLower(assetName), ".tar") || strings.HasSuffix(strings.ToLower(assetName), ".zip") {
			return extractSingleFileAsset(downloadPath, assetName, targetPath)
		}
		return copyFile(downloadPath, targetPath)
	case ArtifactDirectory:
		switch {
		case strings.HasSuffix(strings.ToLower(assetName), ".tar"):
			return extractTarArchive(downloadPath, targetPath)
		case isGzipTarAsset(assetName):
			return extractGzipTarArchive(downloadPath, targetPath)
		case strings.HasSuffix(strings.ToLower(assetName), ".zip"):
			return extractDirectoryFromZip(downloadPath, targetPath)
		default:
			return fmt.Errorf("directory artifact %s requires a tar/tar.gz/zip release asset", targetPath)
		}
	default:
		return fmt.Errorf("unsupported artifact kind %s", kind)
	}
}

func isGzipTarAsset(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
}

func extractSingleFileAsset(archivePath, assetName, targetPath string) error {
	lower := strings.ToLower(assetName)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		tempDir, err := os.MkdirTemp("", "wailsrel-delta-zip-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		if err := extractZipArchive(archivePath, tempDir); err != nil {
			return err
		}
		return copySingleExtractedFile(tempDir, targetPath)
	case strings.HasSuffix(lower, ".tar"), isGzipTarAsset(lower):
		tempDir, err := os.MkdirTemp("", "wailsrel-delta-tar-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		if strings.HasSuffix(lower, ".tar") {
			if err := extractTarArchive(archivePath, tempDir); err != nil {
				return err
			}
		} else {
			if err := extractGzipTarArchive(archivePath, tempDir); err != nil {
				return err
			}
		}
		return copySingleExtractedFile(tempDir, targetPath)
	default:
		return copyFile(archivePath, targetPath)
	}
}

func extractDirectoryFromZip(zipPath, targetPath string) error {
	tempDir, err := os.MkdirTemp("", "wailsrel-delta-zip-dir-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := extractZipArchive(zipPath, tempDir); err != nil {
		return err
	}

	base := filepath.Base(targetPath)
	candidate := filepath.Join(tempDir, base)
	if _, err := os.Stat(candidate); err == nil {
		return copyPath(candidate, targetPath)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return copyPath(filepath.Join(tempDir, entries[0].Name()), targetPath)
	}
	return fmt.Errorf("zip asset does not contain %s", base)
}

func copySingleExtractedFile(root, targetPath string) error {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) != 1 {
		return fmt.Errorf("expected exactly one file in extracted asset, found %d", len(files))
	}
	return copyFile(files[0], targetPath)
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := dst
			if rel != "." {
				target = filepath.Join(dst, rel)
			}
			if info.IsDir() {
				return os.MkdirAll(target, info.Mode().Perm())
			}
			return copyFile(path, target)
		})
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
