package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Eriyc/wailsrel/pkg/config"
	projecttemplates "github.com/Eriyc/wailsrel/templates"
	"github.com/spf13/cobra"
)

type initData struct {
	AppName              string
	Identifier           string
	HasDarwinTaskfile    bool
	HasWindowsTaskfile   bool
	HasLinuxTaskfile     bool
	HasDetectedTaskfiles bool
}

func newInitCmd(opts *Options) *cobra.Command {
	var (
		force      bool
		appName    string
		identifier string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold wailsrel.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			outputPath := config.DefaultFileName
			if opts.ConfigPath != "" {
				outputPath = opts.ConfigPath
			}

			absPath, err := filepath.Abs(outputPath)
			if err != nil {
				return err
			}

			if !force {
				if _, err := os.Stat(absPath); err == nil {
					return fmt.Errorf("%s already exists; use --force to overwrite", absPath)
				}
			}

			if appName == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				appName = strings.TrimSpace(filepath.Base(cwd))
			}
			if identifier == "" {
				identifier = "com.example." + slug(appName)
			}

			tmpl, err := template.ParseFS(projecttemplates.FS, "wailsrel.yaml.tmpl")
			if err != nil {
				return err
			}

			var buf bytes.Buffer
			taskfiles := detectWailsTaskfiles(filepath.Dir(absPath))
			if err := tmpl.Execute(&buf, initData{
				AppName:              appName,
				Identifier:           identifier,
				HasDarwinTaskfile:    taskfiles["darwin"],
				HasWindowsTaskfile:   taskfiles["windows"],
				HasLinuxTaskfile:     taskfiles["linux"],
				HasDetectedTaskfiles: taskfiles["darwin"] || taskfiles["windows"] || taskfiles["linux"],
			}); err != nil {
				return err
			}

			if opts.DryRun {
				_, err := fmt.Fprint(cmd.OutOrStdout(), buf.String())
				return err
			}

			if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
				return err
			}

			if err := os.WriteFile(absPath, buf.Bytes(), 0o644); err != nil {
				return err
			}

			if opts.JSON {
				return writeJSON(cmd.OutOrStdout(), map[string]string{
					"config_path": absPath,
				})
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", absPath)
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing config file")
	cmd.Flags().StringVar(&appName, "name", "", "Application name to write into the config")
	cmd.Flags().StringVar(&identifier, "identifier", "", "Reverse-domain application identifier")

	return cmd
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "")
	return strings.ReplaceAll(s, "-", "")
}

func detectWailsTaskfiles(root string) map[string]bool {
	result := map[string]bool{
		"darwin":  false,
		"windows": false,
		"linux":   false,
	}
	for platform := range result {
		_, err := os.Stat(filepath.Join(root, "build", platform, "Taskfile.yml"))
		result[platform] = err == nil
	}
	return result
}
