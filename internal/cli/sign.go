package cli

import "github.com/spf13/cobra"

func newSignCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "sign <path>",
		Short: "Sign a single artifact",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printPlaceholder("sign")
		},
	}
}
