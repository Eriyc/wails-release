package cli

import "github.com/spf13/cobra"

func newDeltaCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delta",
		Short: "Generate delta patches for release artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printPlaceholder("delta")
		},
	}
}
