package cli

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
	"github.com/you/wailsrel/pkg/frontend"
)

func newChannelCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "channel <name>",
		Short: "Switch the active frontend channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, configPath, err := loadConfig(opts)
			if err != nil {
				return err
			}

			channel := args[0]
			if !slices.Contains(cfg.Frontend.Channels, channel) {
				return fmt.Errorf("unknown channel %q", channel)
			}

			manager := frontend.BundleManager{
				AppID:        cfg.App.Identifier,
				NativeCompat: "",
				OverrideRoot: filepath.Join(filepath.Dir(configPath), ".wailsrel"),
			}
			if err := manager.SetChannel(channel); err != nil {
				return err
			}

			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), map[string]string{"channel": channel})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Active frontend channel: %s\n", channel)
			return err
		},
	}
}
