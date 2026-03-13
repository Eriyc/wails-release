package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Eriyc/wailsrel/pkg/build"
)

const artifactMetadataName = ".wailsrel-artifacts.json"

func artifactMetadataPath(outputDir string) string {
	return filepath.Join(outputDir, artifactMetadataName)
}

func writeArtifactMetadata(outputDir string, artifacts []build.Artifact) error {
	data, err := json.MarshalIndent(artifacts, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(artifactMetadataPath(outputDir), data, 0o644)
}

func readArtifactMetadata(outputDir string) ([]build.Artifact, error) {
	data, err := os.ReadFile(artifactMetadataPath(outputDir))
	if err != nil {
		return nil, err
	}
	var artifacts []build.Artifact
	if err := json.Unmarshal(data, &artifacts); err != nil {
		return nil, fmt.Errorf("parse %s: %w", artifactMetadataPath(outputDir), err)
	}
	return artifacts, nil
}
