package cli

import (
	"testing"

	"github.com/you/wailsrel/pkg/config"
)

func TestCheckFrontendCompatSkipsMissingBindingsWithoutSnapshot(t *testing.T) {
	repo := t.TempDir()
	result, err := checkFrontendCompat(repo, &config.Config{
		Frontend: config.FrontendConfig{
			Enabled:         true,
			CompatVersion:   1,
			CompatAutoCheck: true,
			BindingsDir:     "frontend/bindings",
		},
	})
	if err != nil {
		t.Fatalf("check frontend compat: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result when bindings are absent and no snapshot exists, got %+v", result)
	}
}
