package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Name    string `json:"name"`
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
}

type doctorView struct {
	Checks []doctorCheck `json:"checks"`
}

func newDoctorCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local dependencies for configured targets",
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := []doctorCheck{
				lookup("go"),
				lookup("git"),
			}

			cfg, configPath, err := loadConfig(opts)
			if err == nil {
				projectDir := filepath.Dir(configPath)
				seen := map[string]struct{}{
					"go":  {},
					"git": {},
				}
				for _, target := range build.ExpandMatrix(cfg.Targets) {
					for _, tool := range build.RequiredTools(target) {
						if _, ok := seen[tool]; ok {
							continue
						}
						seen[tool] = struct{}{}
						checks = append(checks, lookup(tool))
					}
					if len(target.Build.Argv) == 0 {
						checks = append(checks, doctorCheck{Name: target.ID, Message: "build argv is empty"})
						continue
					}
					if err := validateDoctorTarget(projectDir, target); err != nil {
						checks = append(checks, doctorCheck{Name: target.ID, Message: err.Error()})
						continue
					}
					checks = append(checks, doctorCheck{Name: target.ID, Found: true})
				}
			}

			view := doctorView{Checks: checks}
			if opts.JSON {
				if err := writeJSON(cmd.OutOrStdout(), view); err != nil {
					return err
				}
			} else {
				for _, check := range checks {
					state := "ok"
					if !check.Found {
						state = "missing"
					}
					if check.Message != "" {
						state += " (" + check.Message + ")"
					}
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s\n", check.Name, state); err != nil {
						return err
					}
				}
			}

			for _, check := range checks {
				if !check.Found {
					return errors.New("one or more required dependencies are missing")
				}
			}
			return nil
		},
	}
}

func validateDoctorTarget(projectDir string, target build.Target) error {
	if len(target.Build.Argv) == 0 || strings.TrimSpace(target.Build.Argv[0]) == "" {
		return fmt.Errorf("build.argv must not be empty")
	}
	if strings.TrimSpace(target.Build.Workdir) != "" {
		workdir := target.Build.Workdir
		if !filepath.IsAbs(workdir) {
			workdir = filepath.Join(projectDir, workdir)
		}
		if _, err := exec.LookPath(target.Build.Argv[0]); err != nil {
			return err
		}
		if _, err := filepath.Abs(workdir); err != nil {
			return err
		}
	}
	for i, artifact := range target.Artifacts {
		pathSet := strings.TrimSpace(artifact.Path) != ""
		globSet := strings.TrimSpace(artifact.Glob) != ""
		switch {
		case pathSet && globSet:
			return fmt.Errorf("artifact %d has both path and glob", i)
		case !pathSet && !globSet:
			return fmt.Errorf("artifact %d requires path or glob", i)
		}
	}
	return nil
}

func lookup(name string) doctorCheck {
	path, err := exec.LookPath(name)
	if err != nil {
		return doctorCheck{Name: name}
	}
	return doctorCheck{Name: name, Found: true, Path: path}
}
