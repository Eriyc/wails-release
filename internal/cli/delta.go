package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/you/wailsrel/pkg/delta"
)

func newDeltaCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delta",
		Short: "Generate delta patches for release artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, configPath, err := loadConfig(opts)
			if err != nil {
				return err
			}
			if !cfg.Delta.Enabled {
				return fmt.Errorf("delta generation is disabled in config")
			}

			projectDir := filepath.Dir(configPath)
			repository := cfg.Delta.OldArtifacts.Repository
			if cfg.Delta.OldArtifacts.Source == "github-release" {
				repository, err = resolveGitHubRepository(projectDir, repository)
				if err != nil {
					return err
				}
			}

			generatorOpts := deltaOptions(projectDir, cfg, repository)
			generator := delta.NewGenerator(generatorOpts)

			plan, err := generator.PlanContext(cmd.Context())
			if err != nil {
				return err
			}

			if opts.DryRun {
				if opts.JSON {
					return writeJSON(cmd.OutOrStdout(), plan)
				}

				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Delta source: %s\nCache dir: %s\nVersions: %v\n", plan.Source, plan.CacheDir, plan.Versions); err != nil {
					return err
				}
				for _, pending := range plan.Pending {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "plan %s <- %s (%s)\n", pending.Artifact, pending.FromVersion, pending.Patch); err != nil {
						return err
					}
				}
				for _, skipped := range plan.Skipped {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skip %s %s (%s)\n", skipped.FromVersion, skipped.Artifact, skipped.Reason); err != nil {
						return err
					}
				}
				return nil
			}

			result, err := generator.GenerateContext(cmd.Context(), plan)
			if err != nil {
				return err
			}

			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), result)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Generated %d patch(es)\n", len(result.Generated)); err != nil {
				return err
			}
			for _, generated := range result.Generated {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"%s <- %s %s (patch %s vs full %s, %.1f%% smaller)\n",
					generated.Artifact,
					generated.FromVersion,
					generated.Patch,
					humanSize(generated.Size),
					humanSize(generated.ToSize),
					generated.SavingsPercent,
				); err != nil {
					return err
				}
			}
			for _, skipped := range result.Skipped {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skip %s %s (%s)\n", skipped.FromVersion, skipped.Artifact, skipped.Reason); err != nil {
					return err
				}
			}
			if result.ManifestPath != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Manifest %s\n", result.ManifestPath); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}
