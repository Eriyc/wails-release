package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Eriyc/wailsrel/pkg/config"
	"github.com/Eriyc/wailsrel/pkg/sign"
	"github.com/spf13/cobra"
)

type signView struct {
	Path      string `json:"path"`
	OS        string `json:"os"`
	Provider  string `json:"provider"`
	Signed    bool   `json:"signed"`
	Notarized bool   `json:"notarized"`
	Duration  string `json:"duration"`
}

func newSignCmd(opts *Options) *cobra.Command {
	var (
		targetOS string
		provider string
	)

	cmd := &cobra.Command{
		Use:   "sign <path>",
		Short: "Sign a single artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactPath, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}

			cfg, _, err := loadConfig(opts)
			if err != nil {
				return err
			}

			signCfg, resolvedOS, err := resolveSignConfig(cfg, artifactPath, targetOS, provider)
			if err != nil {
				return err
			}

			signer, err := sign.NewSigner(signCfg)
			if err != nil {
				return err
			}
			if err := signer.Available(context.Background()); err != nil {
				return err
			}

			result, err := signer.Sign(context.Background(), artifactPath, sign.SignOpts{
				Identity: signCfg.Identity,
				Notarize: signCfg.Notarize && strings.EqualFold(filepath.Ext(artifactPath), ".dmg"),
				Credentials: map[string]string{
					"identity": signCfg.Identity,
					"apple_id": signCfg.AppleID,
					"password": signCfg.Password,
					"team_id":  signCfg.TeamID,
					"endpoint": signCfg.Endpoint,
					"account":  signCfg.Account,
					"profile":  signCfg.Profile,
				},
			})
			if err != nil {
				return err
			}
			if result.Signed {
				if err := signer.Verify(context.Background(), artifactPath); err != nil {
					return err
				}
			}

			view := signView{
				Path:      artifactPath,
				OS:        resolvedOS,
				Provider:  signer.Provider(),
				Signed:    result.Signed,
				Notarized: result.Notarized,
				Duration:  result.Duration.String(),
			}
			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), view)
			}

			if !result.Signed {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "Skipped signing for %s with %s (%s)\n", artifactPath, signer.Provider(), resolvedOS)
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Signed %s with %s (%s)\n", artifactPath, signer.Provider(), resolvedOS); err != nil {
				return err
			}
			if result.Notarized {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Notarization complete")
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&targetOS, "os", "", "Override target OS detection")
	cmd.Flags().StringVar(&provider, "provider", "", "Override provider selection for the resolved OS")

	return cmd
}

func resolveSignConfig(cfg *config.Config, artifactPath string, overrideOS string, overrideProvider string) (config.SignConfig, string, error) {
	targetOS := strings.TrimSpace(overrideOS)
	if targetOS == "" {
		targetOS = inferTargetOS(artifactPath)
	}
	if targetOS == "" {
		return config.SignConfig{}, "", errors.New("unable to infer target OS from artifact path; use --os")
	}

	var matches []config.SignConfig
	for _, target := range cfg.Targets {
		if target.OS != targetOS {
			continue
		}
		if overrideProvider != "" && target.Sign.Provider != overrideProvider {
			continue
		}
		matches = append(matches, target.Sign)
	}
	if len(matches) == 0 {
		return config.SignConfig{}, "", fmt.Errorf("no signing configuration found for %s", targetOS)
	}

	selected := matches[0]
	for _, match := range matches[1:] {
		if match != selected {
			return config.SignConfig{}, "", fmt.Errorf("multiple signing configurations found for %s; use --provider", targetOS)
		}
	}

	if overrideProvider != "" {
		selected.Provider = overrideProvider
	}

	return selected, targetOS, nil
}

func inferTargetOS(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".app", ".dmg":
		return "darwin"
	case ".exe":
		return "windows"
	case ".appimage", ".deb", ".rpm":
		return "linux"
	default:
		return ""
	}
}
