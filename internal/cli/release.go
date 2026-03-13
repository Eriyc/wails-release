package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	artifactpkg "github.com/you/wailsrel/pkg/artifact"
	"github.com/you/wailsrel/pkg/build"
	"github.com/you/wailsrel/pkg/config"
	"github.com/you/wailsrel/pkg/delta"
	releasepkg "github.com/you/wailsrel/pkg/release"
)

type releaseView struct {
	Provider         string   `json:"provider"`
	Repository       string   `json:"repository"`
	Tag              string   `json:"tag"`
	Version          string   `json:"version"`
	ManifestURL      string   `json:"manifest_url"`
	DeltaManifestURL string   `json:"delta_manifest_url,omitempty"`
	Uploads          []string `json:"uploads"`
	DryRun           bool     `json:"dry_run"`
	Warnings         []string `json:"warnings,omitempty"`
}

type releaseDeps struct {
	build           func(context.Context, string, *config.Config) ([]build.Artifact, error)
	discoverBuild   func(string) ([]build.Artifact, error)
	generateDelta   func(context.Context, string, *config.Config, string) (*delta.Result, error)
	discoverDelta   func(string) (*delta.Result, error)
	prepareBundle   func(releasepkg.BundleOptions) (*releasepkg.Bundle, error)
	publish         func(context.Context, *config.Config, string, string, []releasepkg.UploadAsset) error
	resolveResolver func(*config.Config, string) releasepkg.Resolver
}

func newReleaseCmd(opts *Options) *cobra.Command {
	return newReleaseCmdWithDeps(opts, defaultReleaseDeps())
}

func newReleaseCmdWithDeps(opts *Options, deps releaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "release",
		Short: "Run the full release pipeline",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, configPath, err := loadConfig(opts)
			if err != nil {
				return err
			}

			projectDir := filepath.Dir(configPath)
			tag, versionText, err := resolveReleaseVersion(cmd.Context(), projectDir, cfg)
			if err != nil {
				return err
			}
			compatResult, err := checkFrontendCompat(projectDir, cfg)
			if err != nil {
				return err
			}

			repository, err := resolveGitHubRepository(projectDir, firstNonEmpty(cfg.Release.GitHub.Repository, cfg.Delta.OldArtifacts.Repository))
			if err != nil {
				return err
			}
			resolver := deps.resolveResolver(cfg, repository)

			var artifacts []build.Artifact
			var deltaResult *delta.Result
			if opts.DryRun {
				artifacts, err = deps.discoverBuild(projectOutputDir(projectDir, cfg))
				if err != nil {
					return err
				}
				if cfg.Delta.Enabled {
					deltaResult, err = deps.discoverDelta(projectOutputDir(projectDir, cfg))
					if err != nil {
						return err
					}
				}
			} else {
				artifacts, err = deps.build(cmd.Context(), projectDir, cfg)
				if err != nil {
					return err
				}
				if cfg.Delta.Enabled {
					deltaResult, err = deps.generateDelta(cmd.Context(), projectDir, cfg, repository)
					if err != nil {
						return err
					}
				}
			}

			bundle, err := deps.prepareBundle(releasepkg.BundleOptions{
				App:       cfg.App,
				OutputDir: projectOutputDir(projectDir, cfg),
				TempDir:   filepath.Join(projectDir, ".wailsrel", "release"),
				Tag:       tag,
				Version:   versionText,
				Resolver:  resolver,
				Artifacts: artifacts,
				Delta:     deltaResult,
			})
			if err != nil {
				return err
			}

			view := releaseView{
				Provider:    resolver.Provider(),
				Repository:  repository,
				Tag:         tag,
				Version:     versionText,
				ManifestURL: resolver.ManifestURL(tag),
				Uploads:     uploadNames(bundle.Uploads),
				DryRun:      opts.DryRun,
			}
			if compatResult != nil {
				view.Warnings = append(view.Warnings, compatResult.Warnings...)
			}
			if bundle.DeltaManifestPath != "" {
				view.DeltaManifestURL = resolver.DeltaManifestURL(tag)
			}

			if opts.DryRun {
				if opts.JSON {
					return writeJSON(cmd.OutOrStdout(), view)
				}
				if err := emitWarnings(cmd.ErrOrStderr(), view.Warnings); err != nil {
					return err
				}
				_, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"Provider: %s\nRepository: %s\nTag: %s\nManifest: %s\nDelta manifest: %s\nUploads:\n",
					view.Provider,
					view.Repository,
					view.Tag,
					view.ManifestURL,
					view.DeltaManifestURL,
				)
				if err != nil {
					return err
				}
				for _, name := range view.Uploads {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", name); err != nil {
						return err
					}
				}
				return nil
			}

			if err := deps.publish(cmd.Context(), cfg, repository, tag, bundle.Uploads); err != nil {
				return err
			}
			if err := persistFrontendCompat(projectDir, cfg, compatResult); err != nil {
				return err
			}

			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), view)
			}
			if err := emitWarnings(cmd.ErrOrStderr(), view.Warnings); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Published %d asset(s) for %s\nManifest %s\n", len(bundle.Uploads), tag, view.ManifestURL)
			return err
		},
	}
}

func defaultReleaseDeps() releaseDeps {
	return releaseDeps{
		build:         executeReleaseBuild,
		discoverBuild: discoverReleaseArtifacts,
		generateDelta: executeReleaseDelta,
		discoverDelta: discoverReleaseDelta,
		prepareBundle: releasepkg.PrepareBundle,
		publish:       publishReleaseAssets,
		resolveResolver: func(cfg *config.Config, repository string) releasepkg.Resolver {
			if cfg.Release.Provider == releasepkg.ProviderHTTP {
				return releasepkg.NewHTTPResolver(
					cfg.Release.HTTP.BaseURL,
					cfg.Release.HTTP.ManifestPath,
					cfg.Release.HTTP.DeltaManifestPath,
					cfg.Release.HTTP.DownloadPathPrefix,
				)
			}
			return releasepkg.NewGitHubResolver(repository, cfg.Release.GitHub.APIBaseURL)
		},
	}
}

func executeReleaseBuild(ctx context.Context, projectDir string, cfg *config.Config) ([]build.Artifact, error) {
	outputDir := projectOutputDir(projectDir, cfg)
	if err := artifactpkg.PrepareOutputDir(outputDir, cfg.Output.Clean); err != nil {
		return nil, err
	}

	timeout, err := time.ParseDuration(cfg.CI.Timeout)
	if err != nil {
		return nil, err
	}
	builder := build.NewBuilder(build.Options{
		ProjectDir: projectDir,
		OutputDir:  outputDir,
		AppName:    cfg.App.Name,
		Installers: cfg.Installers,
		Timeout:    timeout,
	})

	matrix := build.ExpandMatrix(cfg.Targets)
	var artifacts []build.Artifact
	for _, target := range matrix {
		if err := builder.Available(ctx, target); err != nil {
			return nil, err
		}
		result, err := builder.Build(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("build %s/%s: %w", target.OS, target.Arch, err)
		}
		artifacts = append(artifacts, result.Artifacts...)
	}
	return artifacts, nil
}

func executeReleaseDelta(ctx context.Context, projectDir string, cfg *config.Config, repository string) (*delta.Result, error) {
	generator := delta.NewGenerator(deltaOptions(projectDir, cfg, repository))
	plan, err := generator.PlanContext(ctx)
	if err != nil {
		return nil, err
	}
	return generator.GenerateContext(ctx, plan)
}

func publishReleaseAssets(ctx context.Context, cfg *config.Config, repository, tag string, uploads []releasepkg.UploadAsset) error {
	token := os.Getenv("GITHUB_TOKEN")
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("GITHUB_TOKEN must be set to publish release assets")
	}
	client := releasepkg.NewGitHubClient(cfg.Release.GitHub.APIBaseURL, token, nil)
	_, err := client.ReplaceAssets(ctx, repository, tag, uploads)
	return err
}

func uploadNames(uploads []releasepkg.UploadAsset) []string {
	names := make([]string, 0, len(uploads))
	for _, upload := range uploads {
		names = append(names, upload.Name)
	}
	return names
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
