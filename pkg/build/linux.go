package build

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
)

func (b *WailsBuilder) buildLinux(ctx context.Context, target Target) ([]Artifact, error) {
	needsBinary := slices.Contains(target.OutputFormats, "binary") || slices.Contains(target.OutputFormats, "zip")
	needsPackage := slices.Contains(target.OutputFormats, "appimage") || slices.Contains(target.OutputFormats, "deb") || slices.Contains(target.OutputFormats, "rpm")

	if needsBinary {
		if err := b.wailsBuild(ctx, target); err != nil {
			return nil, err
		}
	}
	if needsPackage {
		if err := b.wailsPackage(ctx, target); err != nil {
			return nil, err
		}
	}

	binaryPath := filepath.Join(b.projectDir, "bin", b.appName)
	var artifacts []Artifact
	for _, format := range target.OutputFormats {
		switch format {
		case "binary":
			if err := requirePath(binaryPath); err != nil {
				return nil, err
			}
			artifact, err := b.stageArtifact(target, "binary", binaryPath, nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		case "appimage":
			path, err := findArtifact(filepath.Join(b.projectDir, "bin"), "*.AppImage", "*"+target.Arch+"*.AppImage")
			if err != nil {
				return nil, err
			}
			artifact, err := b.stageArtifact(target, "appimage", path, nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		case "deb":
			path, err := findArtifact(filepath.Join(b.projectDir, "bin"), "*.deb", "*"+target.Arch+"*.deb")
			if err != nil {
				return nil, err
			}
			artifact, err := b.stageArtifact(target, "deb", path, nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		case "rpm":
			path, err := findArtifact(filepath.Join(b.projectDir, "bin"), "*.rpm", "*"+target.Arch+"*.rpm")
			if err != nil {
				return nil, err
			}
			artifact, err := b.stageArtifact(target, "rpm", path, nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		case "zip":
			if err := requirePath(binaryPath); err != nil {
				return nil, err
			}
			artifact, err := b.stageZip(target, binaryPath)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		default:
			if !slices.Contains([]string{"binary", "appimage", "deb", "rpm", "zip"}, format) {
				return nil, fmt.Errorf("unsupported linux output format %q", format)
			}
		}
	}

	return artifacts, nil
}
