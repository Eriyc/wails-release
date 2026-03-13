package cli

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/Eriyc/wailsrel/pkg/config"
	"github.com/Eriyc/wailsrel/pkg/frontend"
)

func executeReleaseFrontendBundles(ctx context.Context, projectDir string, cfg *config.Config, versionText string) ([]frontend.BundleArtifact, error) {
	if !cfg.Frontend.Enabled {
		return nil, nil
	}

	outputDir := projectOutputDir(projectDir, cfg)
	bundles := make([]frontend.BundleArtifact, 0, len(cfg.Frontend.Channels))
	for _, channel := range cfg.Frontend.Channels {
		outputPath := filepath.Join(outputDir, fmt.Sprintf("frontend-%s-%s.zip", channel, versionText))
		bundle, err := frontend.BuildBundle(ctx, frontend.BundleOpts{
			WorkDir:       projectDir,
			OutputPath:    outputPath,
			OutputDir:     outputDir,
			BuildCommand:  cfg.Frontend.BuildCommand,
			BuildDir:      cfg.Frontend.BuildDir,
			BindingsDir:   cfg.Frontend.BindingsDir,
			CompatVer:     strconv.Itoa(cfg.Frontend.CompatVersion),
			CompatVersion: cfg.Frontend.CompatVersion,
			Channel:       channel,
			Version:       versionText,
		})
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, *bundle)
	}

	sort.Slice(bundles, func(i, j int) bool {
		return bundles[i].Manifest.Channel < bundles[j].Manifest.Channel
	})
	return bundles, nil
}

func discoverReleaseFrontendBundles(outputDir string) ([]frontend.BundleArtifact, error) {
	paths, err := filepath.Glob(filepath.Join(outputDir, "frontend-*.zip"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	bundles := make([]frontend.BundleArtifact, 0, len(paths))
	for _, path := range paths {
		manifest, err := readFrontendBundleManifest(path)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, frontend.BundleArtifact{
			Path:     path,
			Manifest: *manifest,
			Size:     info.Size(),
		})
	}
	return bundles, nil
}

func readFrontendBundleManifest(bundlePath string) (*frontend.BundleManifest, error) {
	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name != "bundle.json" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		var manifest frontend.BundleManifest
		if err := json.NewDecoder(rc).Decode(&manifest); err != nil {
			return nil, err
		}
		return &manifest, nil
	}
	return nil, fmt.Errorf("bundle.json not found in archive %s", bundlePath)
}
