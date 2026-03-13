package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Eriyc/wailsrel/pkg/config"
	"github.com/Eriyc/wailsrel/pkg/delta"
	"github.com/Eriyc/wailsrel/pkg/release"
	"github.com/Eriyc/wailsrel/pkg/version"
)

func projectOutputDir(projectDir string, cfg *config.Config) string {
	outputDir := cfg.Output.Dir
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(projectDir, outputDir)
	}
	return outputDir
}

func projectCacheDir(projectDir string, cfg *config.Config) string {
	cacheDir := cfg.Delta.OldArtifacts.CacheDir
	if !filepath.IsAbs(cacheDir) {
		cacheDir = filepath.Join(projectDir, cacheDir)
	}
	return cacheDir
}

func resolveReleaseVersion(ctx context.Context, projectDir string, cfg *config.Config) (string, string, error) {
	switch cfg.Version.Source {
	case "git":
		manager := version.NewGitManager(projectDir, cfg.Version.TagPrefix)
		tag, v, err := manager.Latest(ctx)
		if err != nil {
			return "", "", err
		}
		return tag, v.String(), nil
	case "file":
		path := cfg.Version.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", "", err
		}
		raw := strings.TrimSpace(string(data))
		v, err := version.Parse(raw)
		if err != nil {
			return "", "", err
		}
		return cfg.Version.TagPrefix + v.String(), v.String(), nil
	default:
		return "", "", fmt.Errorf("unsupported version source %q", cfg.Version.Source)
	}
}

func resolveGitHubRepository(projectDir, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}

	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve git origin remote: %w", err)
	}

	repository, err := parseGitHubRepository(strings.TrimSpace(string(output)))
	if err != nil {
		return "", err
	}
	return repository, nil
}

func parseGitHubRepository(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	switch {
	case strings.HasPrefix(remote, "git@"):
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("unsupported git remote %q", remote)
		}
		return strings.TrimSuffix(parts[1], ".git"), nil
	case strings.Contains(remote, "://"):
		parsed, err := url.Parse(remote)
		if err != nil {
			return "", err
		}
		path := strings.TrimPrefix(parsed.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		if path == "" {
			return "", fmt.Errorf("unsupported git remote %q", remote)
		}
		return path, nil
	default:
		return "", fmt.Errorf("unsupported git remote %q", remote)
	}
}

func deltaOptions(projectDir string, cfg *config.Config, repository string) delta.Options {
	outputDir := projectOutputDir(projectDir, cfg)
	return delta.Options{
		OutputDir:        outputDir,
		CacheDir:         projectCacheDir(projectDir, cfg),
		Artifacts:        deltaArtifactPaths(outputDir),
		FromVersions:     cfg.Delta.FromVersions,
		Source:           cfg.Delta.OldArtifacts.Source,
		TagPrefix:        cfg.Version.TagPrefix,
		Repository:       repository,
		ManifestURL:      resolvedDeltaManifestURL(cfg),
		AuthToken:        os.Getenv(strings.TrimSpace(cfg.Delta.OldArtifacts.AuthTokenEnv)),
		GitHubAPIBaseURL: cfg.Release.GitHub.APIBaseURL,
	}
}

func deltaArtifactPaths(outputDir string) []string {
	artifacts, err := readArtifactMetadata(outputDir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.EnableDelta {
			paths = append(paths, artifact.Path)
		}
	}
	return paths
}

func resolvedDeltaManifestURL(cfg *config.Config) string {
	if value := strings.TrimSpace(cfg.Delta.OldArtifacts.ManifestURL); value != "" {
		return value
	}
	if value := strings.TrimSpace(cfg.Update.ManifestURL); value != "" {
		return value
	}
	if cfg.Release.Provider == release.ProviderHTTP && strings.TrimSpace(cfg.Release.HTTP.BaseURL) != "" {
		resolver := release.NewHTTPResolver(
			cfg.Release.HTTP.BaseURL,
			cfg.Release.HTTP.ManifestPath,
			cfg.Release.HTTP.DeltaManifestPath,
			cfg.Release.HTTP.DownloadPathPrefix,
		)
		return resolver.ManifestURL("")
	}
	return ""
}
