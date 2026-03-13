package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type statusView struct {
	ConfigPath string   `json:"config_path"`
	AppName    string   `json:"app_name"`
	Identifier string   `json:"identifier"`
	Targets    int      `json:"targets"`
	Channels   []string `json:"channels"`
	OutputDir  string   `json:"output_dir"`
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

			view := statusView{
				ConfigPath: path,
				AppName:    cfg.App.Name,
				Identifier: cfg.App.Identifier,
				Targets:    len(cfg.Targets),
				Channels:   cfg.Frontend.Channels,
				OutputDir:  cfg.Output.Dir,
			}

			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), view)
			}

			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Config: %s\nApp: %s (%s)\nTargets: %d\nFrontend channels: %s\nOutput dir: %s\n",
				view.ConfigPath,
				view.AppName,
				view.Identifier,
				view.Targets,
				strings.Join(view.Channels, ", "),
				view.OutputDir,
			)
			return err
		},
	}
}
