package cli

import "github.com/spf13/cobra"

func newBundleCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "bundle",
		Short: "Build a frontend bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printPlaceholder("bundle")
		},
	}
}
