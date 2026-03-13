package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Eriyc/wailsrel/pkg/contract"
	"github.com/Eriyc/wailsrel/pkg/frontend"
	"github.com/spf13/cobra"
)

func newCatalogCmd(_ *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Frontend catalog helpers",
	}
	cmd.AddCommand(newCatalogVerifyCmd())
	return cmd
}

func newCatalogVerifyCmd() *cobra.Command {
	var path string
	var publicKey string
	var appID string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a signed frontend catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			if path == "" || publicKey == "" {
				return fmt.Errorf("--path and --public-key are required")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			contentType := contract.ContentTypeJSON
			if filepath.Ext(path) == ".pb" {
				contentType = contract.ContentTypeProtobuf
			}
			catalog, err := frontend.DecodeCatalogResponse(data, contentType, publicKey, appID)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "verified app_id=%s codepush=%d experiments=%d\n", catalog.AppID, len(catalog.Codepush), len(catalog.Experiments))
			return err
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Catalog path (.json or .pb)")
	cmd.Flags().StringVar(&publicKey, "public-key", "", "Ed25519 public key")
	cmd.Flags().StringVar(&appID, "app-id", "", "Expected application ID")
	return cmd
}
