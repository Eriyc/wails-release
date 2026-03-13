package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

func (b *WailsBuilder) buildDarwin(ctx context.Context, target Target) ([]Artifact, error) {
	if err := b.wailsPackage(ctx, target); err != nil {
		return nil, err
	}

	appPath := filepath.Join(b.projectDir, "bin", b.appName+".app")
	if err := requirePath(appPath); err != nil {
		return nil, err
	}

	var artifacts []Artifact
	for _, format := range target.OutputFormats {
		switch format {
		case "app":
			artifact, err := b.stageArtifact(target, "app", appPath, nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		case "dmg":
			dmgPath, err := b.createDMG(ctx, target, appPath)
			if err != nil {
				return nil, err
			}
			artifact, err := b.stageArtifact(target, "dmg", dmgPath, nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		case "zip":
			artifact, err := b.stageZip(target, appPath)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		default:
			if !slices.Contains([]string{"app", "dmg", "zip"}, format) {
				return nil, fmt.Errorf("unsupported darwin output format %q", format)
			}
		}
	}

	return artifacts, nil
}

func (b *WailsBuilder) createDMG(ctx context.Context, target Target, appPath string) (string, error) {
	dmgDir := filepath.Join(b.projectDir, ".wailsrel", "tmp", "darwin", target.Arch)
	if err := os.MkdirAll(dmgDir, 0o755); err != nil {
		return "", err
	}

	stagingDir := filepath.Join(dmgDir, "payload")
	if err := os.RemoveAll(stagingDir); err != nil {
		return "", err
	}
	if err := copyPath(appPath, filepath.Join(stagingDir, filepath.Base(appPath))); err != nil {
		return "", err
	}

	dmgPath := filepath.Join(dmgDir, b.appName+"-"+target.Arch+".dmg")
	if err := os.RemoveAll(dmgPath); err != nil {
		return "", err
	}
	if err := b.run(ctx, "hdiutil", "create", "-volname", b.appName, "-srcfolder", stagingDir, "-ov", "-format", "UDZO", dmgPath); err != nil {
		return "", err
	}

	return dmgPath, nil
}
