package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/spf13/cobra"
)

type targetStatus struct {
	ID           string   `json:"id"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	BuildArgv    []string `json:"build_argv"`
	Artifacts    int      `json:"artifacts"`
	Requirements []string `json:"requirements"`
}

type statusView struct {
	ConfigPath          string         `json:"config_path"`
	AppName             string         `json:"app_name"`
	Identifier          string         `json:"identifier"`
	Targets             []targetStatus `json:"targets"`
	Channels            []string       `json:"channels"`
	OutputDir           string         `json:"output_dir"`
	ReleaseProvider     string         `json:"release_provider"`
	CompatAutoCheck     bool           `json:"frontend_compat_auto_check"`
	FrontendBindingsDir string         `json:"frontend_bindings_dir,omitempty"`
}

func newStatusCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show config and current project state",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := loadConfig(opts)
			if err != nil {
				return err
			}

			matrix := build.ExpandMatrix(cfg.Targets)
			targets := make([]targetStatus, 0, len(matrix))
			for _, target := range matrix {
				targets = append(targets, targetStatus{
					ID:           target.ID,
					OS:           target.OS,
					Arch:         target.Arch,
					BuildArgv:    append([]string(nil), target.Build.Argv...),
					Artifacts:    len(target.Artifacts),
					Requirements: build.RequiredTools(target),
				})
			}

			view := statusView{
				ConfigPath:          path,
				AppName:             cfg.App.Name,
				Identifier:          cfg.App.Identifier,
				Targets:             targets,
				Channels:            cfg.Frontend.Channels,
				OutputDir:           cfg.Output.Dir,
				ReleaseProvider:     cfg.Release.Provider,
				CompatAutoCheck:     cfg.Frontend.CompatAutoCheck,
				FrontendBindingsDir: cfg.Frontend.BindingsDir,
			}

			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), view)
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"Config: %s\nApp: %s (%s)\nOutput dir: %s\nRelease provider: %s\nFrontend channels: %s\n",
				view.ConfigPath,
				view.AppName,
				view.Identifier,
				view.OutputDir,
				view.ReleaseProvider,
				strings.Join(view.Channels, ", "),
			); err != nil {
				return err
			}

			for _, target := range view.Targets {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"Target %s: %s/%s -> %s (%d artifact specs)\n",
					target.ID,
					target.OS,
					target.Arch,
					strings.Join(target.BuildArgv, " "),
					target.Artifacts,
				); err != nil {
					return err
				}
			}

			if view.CompatAutoCheck && strings.TrimSpace(view.FrontendBindingsDir) != "" {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Frontend bindings dir: %s\n", filepath.Clean(view.FrontendBindingsDir))
				return err
			}
			return nil
		},
	}
}
