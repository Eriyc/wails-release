package cli

import "github.com/spf13/cobra"

func newReleaseCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "release",
		Short: "Run the full release pipeline",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printPlaceholder("release")
		},
	}
}
