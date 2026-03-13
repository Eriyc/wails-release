package manifest

import (
	"fmt"
	"strings"
	"time"
)

func Generate(opts GenerateOpts) (*ReleaseManifest, error) {
	date := opts.Date
	if date.IsZero() {
		date = time.Now().UTC()
	}

	manifest := &ReleaseManifest{
		SchemaVersion: 1,
		AppName:       strings.TrimSpace(opts.AppName),
		Version:       strings.TrimSpace(opts.Version),
		Channel:       strings.TrimSpace(opts.Channel),
		Date:          date.UTC(),
		ReleaseNotes:  opts.ReleaseNotes,
		NativeCompat:  strings.TrimSpace(opts.NativeCompat),
		Mandatory:     opts.Mandatory,
	}

	manifest.Artifacts = make([]ArtifactEntry, 0, len(opts.Artifacts))
	for _, artifact := range opts.Artifacts {
		url, err := resolveTemplateURL(opts.ArtifactURLTemplate, artifact.URL, templateValues{
			Version: opts.Version,
			OS:      artifact.OS,
			Arch:    artifact.Arch,
			Name:    artifact.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("artifact %s: %w", artifact.Name, err)
		}
		manifest.Artifacts = append(manifest.Artifacts, ArtifactEntry{
			OS:       artifact.OS,
			Arch:     artifact.Arch,
			Format:   artifact.Format,
			URL:      url,
			Checksum: artifact.Checksum,
			Size:     artifact.Size,
		})
	}

	manifest.Patches = make([]PatchEntry, 0, len(opts.Patches))
	for _, patch := range opts.Patches {
		url, err := resolveTemplateURL(opts.PatchURLTemplate, patch.URL, templateValues{
			Version: opts.Version,
			OS:      patch.OS,
			Arch:    patch.Arch,
			Name:    patch.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("patch %s: %w", patch.Name, err)
		}
		manifest.Patches = append(manifest.Patches, PatchEntry{
			FromVersion: patch.FromVersion,
			OS:          patch.OS,
			Arch:        patch.Arch,
			FromHash:    patch.FromHash,
			ToHash:      patch.ToHash,
			URL:         url,
			Checksum:    patch.Checksum,
			Size:        patch.Size,
		})
	}

	manifest.FrontendBundles = make([]FrontendEntry, 0, len(opts.FrontendBundles))
	for _, bundle := range opts.FrontendBundles {
		url, err := resolveTemplateURL(opts.FrontendURLTemplate, bundle.URL, templateValues{
			Version: opts.Version,
			Channel: bundle.Channel,
			Name:    bundle.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("frontend bundle %s: %w", bundle.Name, err)
		}
		manifest.FrontendBundles = append(manifest.FrontendBundles, FrontendEntry{
			Channel:  bundle.Channel,
			Version:  bundle.Version,
			CompatID: bundle.CompatID,
			URL:      url,
			Checksum: bundle.Checksum,
			Size:     bundle.Size,
		})
	}

	if errs := Validate(manifest); len(errs) > 0 {
		return nil, errs[0]
	}
	return manifest, nil
}

type templateValues struct {
	Version string
	OS      string
	Arch    string
	Channel string
	Name    string
}

func resolveTemplateURL(template, explicit string, values templateValues) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit), nil
	}
	if strings.TrimSpace(template) == "" {
		return "", fmt.Errorf("url or template is required")
	}
	replacer := strings.NewReplacer(
		"{version}", values.Version,
		"{os}", values.OS,
		"{arch}", values.Arch,
		"{channel}", values.Channel,
		"{name}", values.Name,
	)
	return replacer.Replace(template), nil
}
