package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/you/wailsrel/pkg/frontend"
)

type bundleView struct {
	Channel       string   `json:"channel"`
	Version       string   `json:"version"`
	CompatVersion int      `json:"compat_version"`
	Path          string   `json:"path"`
	Size          int64    `json:"size,omitempty"`
	DryRun        bool     `json:"dry_run"`
	Warnings      []string `json:"warnings,omitempty"`
}

func newBundleCmd(opts *Options) *cobra.Command {
	var channel string

	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Build a frontend bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, configPath, err := loadConfig(opts)
			if err != nil {
				return err
			}
			if !cfg.Frontend.Enabled {
				return fmt.Errorf("frontend bundling is disabled")
			}

			projectDir := filepath.Dir(configPath)
			channel, err = resolveBundleChannel(projectDir, cfg.Frontend.Channels, channel)
			if err != nil {
				return err
			}

			_, versionText, err := resolveReleaseVersion(cmd.Context(), projectDir, cfg)
			if err != nil {
				return err
			}

			outputPath := filepath.Join(projectOutputDir(projectDir, cfg), fmt.Sprintf("frontend-%s-%s.zip", channel, versionText))
			view := bundleView{
				Channel:       channel,
				Version:       versionText,
				CompatVersion: cfg.Frontend.CompatVersion,
				Path:          outputPath,
				DryRun:        opts.DryRun,
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
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "Frontend bundle: %s\nChannel: %s\nVersion: %s\n", view.Path, view.Channel, view.Version)
				return err
			}

			if err := osMkdirAll(filepath.Dir(outputPath)); err != nil {
				return err
			}

			artifact, err := frontend.BuildBundle(cmd.Context(), frontend.BundleOpts{
				WorkDir:       projectDir,
				OutputPath:    outputPath,
				OutputDir:     projectOutputDir(projectDir, cfg),
				BuildCommand:  cfg.Frontend.BuildCommand,
				BuildDir:      cfg.Frontend.BuildDir,
				BindingsDir:   cfg.Frontend.BindingsDir,
				CompatVer:     strconv.Itoa(cfg.Frontend.CompatVersion),
				CompatVersion: cfg.Frontend.CompatVersion,
				Channel:       channel,
				Version:       versionText,
			})
			if err != nil {
				return err
			}
			view.Path = artifact.Path
			view.Size = artifact.Size

			if err := persistFrontendCompat(projectDir, cfg, compatResult); err != nil {
				return err
			}

			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), view)
			}
			if err := emitWarnings(cmd.ErrOrStderr(), view.Warnings); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Built frontend bundle %s\n", artifact.Path)
			return err
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "Frontend channel to package")
	return cmd
}

func resolveBundleChannel(projectDir string, validChannels []string, requested string) (string, error) {
	requested = firstNonEmpty(requested, activeFrontendChannel(projectDir))
	if requested == "" && len(validChannels) > 0 {
		requested = validChannels[0]
	}
	if requested == "" {
		return "", fmt.Errorf("frontend channel is required")
	}
	if !slices.Contains(validChannels, requested) {
		return "", fmt.Errorf("unknown channel %q", requested)
	}
	return requested, nil
}

func activeFrontendChannel(projectDir string) string {
	manager := frontend.BundleManager{
		OverrideRoot: filepath.Join(projectDir, ".wailsrel"),
	}
	return manager.ActiveChannel()
}

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
