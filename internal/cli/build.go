package cli

import "github.com/spf13/cobra"

func newBuildCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Build configured release targets",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printPlaceholder("build")
		},
	}
}
