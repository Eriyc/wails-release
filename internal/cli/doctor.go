package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/Eriyc/wailsrel/pkg/sign"
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
			toolNames := []string{"go", "git"}

			cfg, _, err := loadConfig(opts)
			if err == nil {
				for _, target := range build.ExpandMatrix(cfg.Targets) {
					for _, tool := range build.RequiredTools(target) {
						toolNames = append(toolNames, tool)
					}
				}
			}

			checks := make([]doctorCheck, 0, len(toolNames))
			seen := make(map[string]struct{}, len(toolNames))
			for _, name := range toolNames {
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				checks = append(checks, lookup(name))
			}
			if err == nil {
				seenSigning := make(map[string]struct{})
				for _, target := range build.ExpandMatrix(cfg.Targets) {
					key := target.OS + "/" + target.Sign.Provider
					if _, ok := seenSigning[key]; ok {
						continue
					}
					seenSigning[key] = struct{}{}

					signer, signErr := sign.NewSigner(target.Sign)
					if signErr != nil {
						checks = append(checks, doctorCheck{
							Name:    "sign " + key,
							Message: signErr.Error(),
						})
						continue
					}

					check := doctorCheck{
						Name:  "sign " + key,
						Found: true,
					}
					if availErr := signer.Available(context.Background()); availErr != nil {
						check.Found = false
						check.Message = availErr.Error()
					}
					checks = append(checks, check)
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
						state = state + " (" + check.Message + ")"
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

func lookup(name string) doctorCheck {
	path, err := exec.LookPath(name)
	if err != nil {
		return doctorCheck{Name: name}
	}

	return doctorCheck{Name: name, Found: true, Path: path}
}
