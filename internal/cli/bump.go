package cli

import "github.com/spf13/cobra"

func newBumpCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "bump <patch|minor|major|pre>",
		Short: "Bump the application version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printPlaceholder("bump")
		},
	}
}
