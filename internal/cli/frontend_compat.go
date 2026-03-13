package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/you/wailsrel/pkg/config"
	"github.com/you/wailsrel/pkg/frontend"
)

type frontendCompatResult struct {
	BindingsHash string
	Warnings     []string
}

func checkFrontendCompat(projectDir string, cfg *config.Config) (*frontendCompatResult, error) {
	if !cfg.Frontend.Enabled || !cfg.Frontend.CompatAutoCheck {
		return nil, nil
	}

	bindingsDir := cfg.Frontend.BindingsDir
	if !filepath.IsAbs(bindingsDir) {
		bindingsDir = filepath.Join(projectDir, bindingsDir)
	}
	hash, err := frontend.ComputeCompatHash(bindingsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !frontendCompatSnapshotExists(projectDir) {
			return nil, nil
		}
		return &frontendCompatResult{
			Warnings: []string{fmt.Sprintf("frontend compat check skipped: %v", err)},
		}, nil
	}

	warning, err := frontend.CheckCompatSnapshot(frontendCompatSnapshotPath(projectDir), cfg.Frontend.CompatVersion, hash)
	if err != nil {
		return nil, err
	}

	result := &frontendCompatResult{BindingsHash: hash}
	if strings.TrimSpace(warning) != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return result, nil
}

func persistFrontendCompat(projectDir string, cfg *config.Config, result *frontendCompatResult) error {
	if result == nil || result.BindingsHash == "" || !cfg.Frontend.Enabled || !cfg.Frontend.CompatAutoCheck {
		return nil
	}
	return frontend.WriteCompatSnapshot(frontendCompatSnapshotPath(projectDir), frontend.CompatSnapshot{
		CompatVersion: cfg.Frontend.CompatVersion,
		BindingsHash:  result.BindingsHash,
	})
}

func emitWarnings(w io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if strings.TrimSpace(warning) == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func frontendCompatSnapshotPath(projectDir string) string {
	return filepath.Join(projectDir, ".wailsrel", "frontend-compat.json")
}

func frontendCompatSnapshotExists(projectDir string) bool {
	_, err := os.Stat(frontendCompatSnapshotPath(projectDir))
	return err == nil
}
