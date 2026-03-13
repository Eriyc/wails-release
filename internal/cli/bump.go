package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/you/wailsrel/pkg/version"
)

func newBumpCmd(opts *Options) *cobra.Command {
	var (
		push       bool
		prerelease bool
	)

	cmd := &cobra.Command{
		Use:   "bump <patch|minor|major|pre>",
		Short: "Bump the application version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, configPath, err := loadConfig(opts)
			if err != nil {
				return err
			}
			if cfg.Version.Source != "git" {
				return fmt.Errorf("bump only supports version.source=git")
			}

			bumpType, err := parseBumpType(args[0])
			if err != nil {
				return err
			}
			if prerelease && bumpType == version.BumpPrerelease {
				return fmt.Errorf("--prerelease cannot be combined with %q", args[0])
			}

			repoDir := filepath.Dir(configPath)
			manager := version.NewGitManager(repoDir, cfg.Version.TagPrefix)

			dirty, err := manager.IsDirty(context.Background())
			if err != nil {
				return err
			}
			if dirty {
				return fmt.Errorf("git working tree is dirty; commit or stash changes before bumping")
			}

			currentTag, currentVersion, err := manager.Latest(context.Background())
			if err != nil && !errors.Is(err, version.ErrNoVersionTags) {
				return err
			}
			if errors.Is(err, version.ErrNoVersionTags) {
				currentTag = ""
				currentVersion = version.Version{}
			}

			nextVersion := currentVersion.Bump(bumpType, cfg.Version.PrereleaseFormat)
			if prerelease {
				nextVersion = nextVersion.WithPrerelease(cfg.Version.PrereleaseFormat)
			}

			commits, err := manager.CommitsSince(context.Background(), currentTag)
			if err != nil {
				return err
			}

			tagName := cfg.Version.TagPrefix + nextVersion.String()
			changelog := version.GenerateChangelog(commits, currentVersion, nextVersion)

			view := map[string]any{
				"current_version": currentVersion.String(),
				"next_version":    nextVersion.String(),
				"tag":             tagName,
				"push":            push,
				"prerelease":      prerelease,
				"dry_run":         opts.DryRun,
			}

			if opts.DryRun {
				if opts.JSON {
					return writeJSON(cmd.OutOrStdout(), view)
				}

				_, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"Current version: %s\nNext version: %s\nTag: %s\nPush: %t\n\n%s\n",
					currentVersion.String(),
					nextVersion.String(),
					tagName,
					push,
					changelog,
				)
				return err
			}

			if err := manager.Tag(context.Background(), nextVersion, changelog, push); err != nil {
				return err
			}

			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), view)
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "created tag %s\n", tagName)
			return err
		},
	}

	cmd.Flags().BoolVar(&push, "push", false, "Push the new tag to origin")
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "Append the configured prerelease format after the bump")

	return cmd
}

func parseBumpType(value string) (version.BumpType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "patch":
		return version.BumpPatch, nil
	case "minor":
		return version.BumpMinor, nil
	case "major":
		return version.BumpMajor, nil
	case "pre", "prerelease":
		return version.BumpPrerelease, nil
	default:
		return 0, fmt.Errorf("unknown bump type %q", value)
	}
}
