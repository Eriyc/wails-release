package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Eriyc/wailsrel/pkg/config"
	"github.com/spf13/cobra"
)

type Options struct {
	ConfigPath string
	Verbose    bool
	JSON       bool
	DryRun     bool
	Color      string
}

func NewRootCommand() *cobra.Command {
	opts := &Options{}

	cmd := &cobra.Command{
		Use:           "wailsrel",
		Short:         "Release tooling for Wails applications",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVarP(&opts.ConfigPath, "config", "c", "", "Path to wailsrel.yaml (default: auto-discover)")
	cmd.PersistentFlags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Verbose logging")
	cmd.PersistentFlags().BoolVar(&opts.JSON, "json", false, "JSON output")
	cmd.PersistentFlags().BoolVar(&opts.DryRun, "dry-run", false, "Show what would be done without doing it")
	cmd.PersistentFlags().StringVar(&opts.Color, "color", "auto", "Force color output (auto, always, never)")

	cmd.AddCommand(
		newInitCmd(opts),
		newDoctorCmd(opts),
		newStatusCmd(opts),
		newBumpCmd(opts),
		newBuildCmd(opts),
		newDeltaCmd(opts),
		newBundleCmd(opts),
		newChannelCmd(opts),
		newReleaseCmd(opts),
	)

	return cmd
}

func loadConfig(opts *Options) (*config.Config, string, error) {
	path := opts.ConfigPath
	if path == "" {
		path = "."
	}

	cfg, err := config.Load(path)
	if err != nil {
		return nil, "", err
	}

	resolved, err := resolveConfigPath(path)
	if err != nil {
		return nil, "", err
	}

	return cfg, resolved, nil
}

func resolveConfigPath(path string) (string, error) {
	if path == "" {
		path = "."
	}

	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return filepath.Abs(filepath.Join(path, config.DefaultFileName))
	}
	if err == nil {
		return filepath.Abs(path)
	}
	if errors.Is(err, os.ErrNotExist) && !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
		return filepath.Abs(filepath.Join(path, config.DefaultFileName))
	}

	return "", err
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printPlaceholder(cmd string) error {
	return fmt.Errorf("%s is not implemented yet", cmd)
}
