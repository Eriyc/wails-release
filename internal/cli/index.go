package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Eriyc/wailsrel/pkg/indexes"
	releasepkg "github.com/Eriyc/wailsrel/pkg/release"
	"github.com/spf13/cobra"
)

func newIndexCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Generate published release/frontend indexes",
	}
	cmd.AddCommand(newReleaseIndexCmd(opts), newFrontendIndexCmd(opts))
	return cmd
}

func newReleaseIndexCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "release",
		Short: "Emit release-index.json and release-index.pb",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, configPath, err := loadConfig(opts)
			if err != nil {
				return err
			}
			projectDir := filepath.Dir(configPath)
			outputDir := projectOutputDir(projectDir, cfg)
			tag, versionText, err := resolveReleaseVersion(cmd.Context(), projectDir, cfg)
			if err != nil {
				return err
			}
			compatResult, err := checkFrontendCompat(projectDir, cfg)
			if err != nil {
				return err
			}

			repository, err := resolveGitHubRepository(projectDir, cfg.Release.GitHub.Repository)
			if err != nil && cfg.Release.Provider == releasepkg.ProviderGitHub {
				return err
			}
			resolver := defaultReleaseDeps().resolveResolver(cfg, repository)

			artifacts, err := discoverReleaseArtifacts(outputDir)
			if err != nil {
				return err
			}
			frontendBundles, err := discoverReleaseFrontendBundles(outputDir)
			if err != nil {
				return err
			}
			bundle, err := releasepkg.PrepareBundle(releasepkg.BundleOptions{
				App:             cfg.App,
				OutputDir:       outputDir,
				TempDir:         filepath.Join(projectDir, ".wailsrel", "release"),
				Tag:             tag,
				Version:         versionText,
				NativeCompatID:  compatBindingsHash(compatResult),
				Resolver:        resolver,
				Artifacts:       artifacts,
				FrontendBundles: frontendBundles,
			})
			if err != nil {
				return err
			}

			index := indexes.BuildReleaseIndex(indexes.ReleaseOptions{
				App:            cfg.App,
				Tag:            tag,
				Version:        versionText,
				PublishedAt:    time.Now().UTC(),
				NativeCompatID: compatBindingsHash(compatResult),
				Artifacts:      bundle.Artifacts,
				Frontend:       bundle.FrontendBundles,
			})
			jsonPath, protoPath, err := indexes.WriteReleaseIndexFiles(outputDir, index)
			if err != nil {
				return err
			}
			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), map[string]string{
					"json": jsonPath,
					"pb":   protoPath,
				})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s and %s\n", jsonPath, protoPath)
			return err
		},
	}
}

func newFrontendIndexCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "frontend",
		Short: "Emit frontend-index.json and frontend-index.pb",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, configPath, err := loadConfig(opts)
			if err != nil {
				return err
			}
			projectDir := filepath.Dir(configPath)
			outputDir := projectOutputDir(projectDir, cfg)
			tag, _, err := resolveReleaseVersion(cmd.Context(), projectDir, cfg)
			if err != nil {
				return err
			}
			bundles, err := discoverReleaseFrontendBundles(outputDir)
			if err != nil {
				return err
			}
			index := indexes.BuildFrontendIndex(indexes.FrontendOptions{
				AppID:       cfg.App.Identifier,
				Tag:         tag,
				PublishedAt: time.Now().UTC(),
				Bundles:     bundles,
			})
			jsonPath, protoPath, err := indexes.WriteFrontendIndexFiles(outputDir, index)
			if err != nil {
				return err
			}
			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), map[string]string{
					"json": jsonPath,
					"pb":   protoPath,
				})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s and %s\n", jsonPath, protoPath)
			return err
		},
	}
}
