package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Eriyc/wailsrel/internal/ci"
	artifactpkg "github.com/Eriyc/wailsrel/pkg/artifact"
	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/spf13/cobra"
)

type buildView struct {
	OutputDir string           `json:"output_dir"`
	Targets   []build.Target   `json:"targets"`
	Artifacts []build.Artifact `json:"artifacts"`
	Duration  string           `json:"duration"`
	Warnings  []string         `json:"warnings,omitempty"`
}

func newBuildCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Build configured release targets",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, configPath, err := loadConfig(opts)
			if err != nil {
				return err
			}

			projectDir := filepath.Dir(configPath)
			outputDir := cfg.Output.Dir
			if !filepath.IsAbs(outputDir) {
				outputDir = filepath.Join(projectDir, outputDir)
			}

			matrix := build.ExpandMatrix(cfg.Targets)
			view := buildView{
				OutputDir: outputDir,
				Targets:   matrix,
			}

			tag, versionText, err := resolveReleaseVersion(cmd.Context(), projectDir, cfg)
			if err != nil {
				return err
			}

			compatResult, err := checkFrontendCompat(projectDir, cfg)
			if err != nil {
				return err
			}
			if compatResult != nil {
				view.Warnings = append(view.Warnings, compatResult.Warnings...)
			}

			if opts.DryRun {
				if opts.JSON {
					return writeJSON(cmd.OutOrStdout(), view)
				}
				if err := emitWarnings(cmd.ErrOrStderr(), view.Warnings); err != nil {
					return err
				}

				_, err := fmt.Fprintf(cmd.OutOrStdout(), "Output dir: %s\nTargets:\n", outputDir)
				if err != nil {
					return err
				}
				for _, target := range matrix {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s/%s) -> %v\n", target.ID, target.OS, target.Arch, target.Build.Argv); err != nil {
						return err
					}
				}
				return nil
			}

			if err := artifactpkg.PrepareOutputDir(outputDir, cfg.Output.Clean); err != nil {
				return err
			}

			timeout, err := time.ParseDuration(cfg.CI.Timeout)
			if err != nil {
				return err
			}

			builder := build.NewBuilder(build.Options{
				ProjectDir: projectDir,
				OutputDir:  outputDir,
				AppName:    cfg.App.Name,
				Version:    versionText,
				Tag:        tag,
				Timeout:    timeout,
			})

			ctx := context.Background()
			start := time.Now()
			for _, target := range matrix {
				if err := builder.Available(ctx, target); err != nil {
					return err
				}

				result, err := builder.Build(ctx, target)
				if err != nil {
					return fmt.Errorf("build %s/%s: %w", target.OS, target.Arch, err)
				}

				view.Artifacts = append(view.Artifacts, result.Artifacts...)
			}
			view.Duration = time.Since(start).String()
			if err := writeArtifactMetadata(outputDir, view.Artifacts); err != nil {
				return err
			}
			if err := persistFrontendCompat(projectDir, cfg, compatResult); err != nil {
				return err
			}

			if info := ci.Detect(); info.IsGitHubActions && cfg.CI.Artifacts.Upload {
				paths := make([]string, 0, len(view.Artifacts))
				for _, artifact := range view.Artifacts {
					paths = append(paths, filepath.Join(outputDir, filepath.FromSlash(artifact.Path)))
				}
				if err := artifactpkg.WriteGitHubOutput(artifactpkg.OutputResult{Artifacts: paths}); err != nil {
					return err
				}
			}

			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), view)
			}
			if err := emitWarnings(cmd.ErrOrStderr(), view.Warnings); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Built %d artifact(s) in %s\n", len(view.Artifacts), view.Duration)
			if err != nil {
				return err
			}
			for _, artifact := range view.Artifacts {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", artifact.Format, filepath.Join(outputDir, filepath.FromSlash(artifact.Path))); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
