package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"

	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Name  string `json:"name"`
	Found bool   `json:"found"`
	Path  string `json:"path,omitempty"`
}

type doctorView struct {
	Checks []doctorCheck `json:"checks"`
}

func newDoctorCmd(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local dependencies for configured targets",
		RunE: func(cmd *cobra.Command, args []string) error {
			var targets []string

			cfg, _, err := loadConfig(opts)
			if err == nil {
				for _, target := range cfg.Targets {
					targets = append(targets, target.OS)
				}
			}

			checks := []doctorCheck{
				lookup("go"),
				lookup("git"),
				lookup("wails3"),
			}

			if slices.Contains(targets, "darwin") {
				checks = append(checks, lookup("codesign"), lookup("hdiutil"))
			}
			if slices.Contains(targets, "windows") {
				checks = append(checks, lookup("makensis"), lookup("signtool"))
			}
			if slices.Contains(targets, "linux") {
				checks = append(checks, lookup("appimagetool"), lookup("dpkg-deb"), lookup("rpmbuild"))
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
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-12s %s\n", check.Name, state); err != nil {
						return err
					}
				}
			}

			for _, check := range checks {
				if !check.Found {
					return errors.New("one or more required tools are missing")
				}
			}

			return nil
		},
	}
}

func lookup(name string) doctorCheck {
	path, err := exec.LookPath(name)
	if err != nil {
		return doctorCheck{Name: name}
	}

	return doctorCheck{Name: name, Found: true, Path: path}
}
