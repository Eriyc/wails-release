package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Eriyc/wailsrel/pkg/contract"
	"github.com/Eriyc/wailsrel/pkg/delta"
	releasepkg "github.com/Eriyc/wailsrel/pkg/release"
	"github.com/spf13/cobra"
)

func newManifestCmd(_ *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Encode manifests as JSON or protobuf",
	}
	cmd.AddCommand(newManifestEncodeCmd())
	return cmd
}

func newManifestEncodeCmd() *cobra.Command {
	var inputPath string
	var outputPath string
	var kind string

	cmd := &cobra.Command{
		Use:   "encode",
		Short: "Re-encode a release or delta manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputPath == "" || outputPath == "" {
				return fmt.Errorf("--input and --output are required")
			}
			data, err := os.ReadFile(inputPath)
			if err != nil {
				return err
			}
			contentType := contract.ContentTypeJSON
			if filepath.Ext(inputPath) == ".pb" {
				contentType = contract.ContentTypeProtobuf
			}

			switch kind {
			case "release":
				manifest, err := releasepkg.DecodeManifest(data, contentType)
				if err != nil {
					return err
				}
				return releasepkg.WriteManifest(manifest, outputPath)
			case "delta":
				manifest, err := delta.DecodeManifest(data, contentType)
				if err != nil {
					return err
				}
				return delta.WriteManifest(manifest, outputPath)
			default:
				return fmt.Errorf("--kind must be release or delta")
			}
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "Manifest kind: release or delta")
	cmd.Flags().StringVar(&inputPath, "input", "", "Input manifest path (.json or .pb)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output manifest path (.json or .pb)")
	return cmd
}
