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
			outputDir := cfg.Output.Dir
			if !filepath.IsAbs(outputDir) {
				outputDir = filepath.Join(projectDir, outputDir)
			}

			cacheDir := cfg.Delta.OldArtifacts.CacheDir
			if !filepath.IsAbs(cacheDir) {
				cacheDir = filepath.Join(projectDir, cacheDir)
			}

			generator := delta.NewGenerator(delta.Options{
				OutputDir:    outputDir,
				CacheDir:     cacheDir,
				FromVersions: cfg.Delta.FromVersions,
				Source:       cfg.Delta.OldArtifacts.Source,
				TagPrefix:    cfg.Version.TagPrefix,
			})

			plan, err := generator.Plan()
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

			result, err := generator.Generate(plan)
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
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s <- %s %s\n", generated.Artifact, generated.FromVersion, generated.Patch); err != nil {
					return err
				}
			}
			for _, skipped := range result.Skipped {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skip %s %s (%s)\n", skipped.FromVersion, skipped.Artifact, skipped.Reason); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
