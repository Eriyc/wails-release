package cli

import (
	"fmt"
	"path/filepath"

	"github.com/Eriyc/wailsrel/pkg/frontend"
	"github.com/spf13/cobra"
)

func newCompatCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compat",
		Short: "Compat ID utilities",
	}
	cmd.AddCommand(newCompatIDCmd(opts))
	return cmd
}

func newCompatIDCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "id [bindings-dir]",
		Short: "Print the deterministic frontend compat ID",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var bindingsDir string
			if len(args) == 1 {
				bindingsDir = args[0]
			} else {
				cfg, configPath, err := loadConfig(opts)
				if err != nil {
					return err
				}
				bindingsDir = cfg.Frontend.BindingsDir
				if !filepath.IsAbs(bindingsDir) {
					bindingsDir = filepath.Join(filepath.Dir(configPath), bindingsDir)
				}
			}

			compatID, err := frontend.CompatID(bindingsDir)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), compatID)
			return err
		},
	}
}
