package cli

import "github.com/spf13/cobra"

func newChannelCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "channel <name>",
		Short: "Switch the active frontend channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printPlaceholder("channel")
		},
	}
}
