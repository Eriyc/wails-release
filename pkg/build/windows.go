package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	projecttemplates "github.com/Eriyc/wailsrel/templates"
)

type nsisTemplateData struct {
	AppName      string
	SourcePath   string
	OutputPath   string
	BinaryName   string
	IconPath     string
	LicensePath  string
	CustomScript string
}

func (b *WailsBuilder) buildWindows(ctx context.Context, target Target) ([]Artifact, error) {
	if err := b.wailsBuild(ctx, target); err != nil {
		return nil, err
	}

	exePath := filepath.Join(b.projectDir, "bin", b.appName+".exe")
	if err := requirePath(exePath); err != nil {
		return nil, err
	}
	if err := b.signArtifact(ctx, target, exePath, false); err != nil {
		return nil, err
	}

	var artifacts []Artifact
	for _, format := range target.OutputFormats {
		switch format {
		case "exe":
			artifact, err := b.stageArtifact(target, "exe", exePath, nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		case "nsis":
			installerPath, err := b.createNSISInstaller(ctx, target, exePath)
			if err != nil {
				return nil, err
			}
			if err := b.signArtifact(ctx, target, installerPath, false); err != nil {
				return nil, err
			}
			artifact, err := b.stageArtifact(target, "nsis", installerPath, nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		case "zip":
			artifact, err := b.stageZip(target, exePath)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		default:
			if !slices.Contains([]string{"exe", "nsis", "zip"}, format) {
				return nil, fmt.Errorf("unsupported windows output format %q", format)
			}
		}
	}

	return artifacts, nil
}

func (b *WailsBuilder) createNSISInstaller(ctx context.Context, target Target, exePath string) (string, error) {
	tmpl, err := b.parseNSISTemplate()
	if err != nil {
		return "", err
	}

	outputDir := filepath.Join(b.projectDir, ".wailsrel", "tmp", "windows", target.Arch)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	scriptPath := filepath.Join(outputDir, "installer.nsi")
	outputPath := filepath.Join(outputDir, b.appName+"-installer.exe")
	data := nsisTemplateData{
		AppName:      b.appName,
		SourcePath:   filepath.Clean(exePath),
		OutputPath:   filepath.Clean(outputPath),
		BinaryName:   filepath.Base(exePath),
		IconPath:     normalizeOptionalPath(b.projectDir, b.installers.NSIS.Icon),
		LicensePath:  normalizeOptionalPath(b.projectDir, b.installers.NSIS.License),
		CustomScript: readOptionalFile(b.projectDir, b.installers.NSIS.CustomScript),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	if err := os.WriteFile(scriptPath, buf.Bytes(), 0o644); err != nil {
		return "", err
	}

	if err := b.run(ctx, "makensis", scriptPath); err != nil {
		return "", err
	}

	return outputPath, requirePath(outputPath)
}

func (b *WailsBuilder) parseNSISTemplate() (*template.Template, error) {
	fs := b.templateFS
	if fs == nil {
		fs = projecttemplates.FS
	}

	data, err := fs.ReadFile("nsis.nsi.tmpl")
	if err != nil {
		return nil, err
	}

	return template.New("nsis.nsi.tmpl").Parse(string(data))
}

func normalizeOptionalPath(root, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return filepath.Clean(filepath.Join(root, value))
}

func readOptionalFile(root, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	data, err := os.ReadFile(filepath.Join(root, value))
	if err != nil {
		return ""
	}

	return string(data)
}
