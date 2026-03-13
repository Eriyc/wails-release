package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteGitHubOutput(result OutputResult) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	artifactDir := ""
	if len(result.Artifacts) > 0 {
		artifactDir = filepath.Dir(result.Artifacts[0])
	}

	lines := []string{
		fmt.Sprintf("manifest_path=%s", result.ManifestPath),
		fmt.Sprintf("artifact_dir=%s", artifactDir),
		fmt.Sprintf("artifact_count=%d", len(result.Artifacts)),
	}

	if _, err := file.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		return err
	}

	return file.Close()
}
