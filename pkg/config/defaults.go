package config

func DefaultConfig() *Config {
	cfg := &Config{}
	cfg.applyDefaults()
	return cfg
}

func (c *Config) applyDefaults() {
	if c.Schema == 0 {
		c.Schema = 2
	}
	if c.Version.Source == "" {
		c.Version.Source = "git"
	}
	if c.Version.TagPrefix == "" {
		c.Version.TagPrefix = "v"
	}
	if c.Version.PrereleaseFormat == "" {
		c.Version.PrereleaseFormat = "beta.{n}"
	}

	if c.Delta.Algorithm == "" {
		c.Delta.Algorithm = "bsdiff"
	}
	if c.Delta.FromVersions == 0 {
		c.Delta.FromVersions = 3
	}
	if c.Delta.OldArtifacts.Source == "" {
		c.Delta.OldArtifacts.Source = "github-release"
	}
	if c.Delta.OldArtifacts.CacheDir == "" {
		c.Delta.OldArtifacts.CacheDir = ".wailsrel/cache"
	}

	if !c.Frontend.Enabled && c.Frontend.BuildDir == "" && c.Frontend.BindingsDir == "" && len(c.Frontend.Channels) == 0 && c.Frontend.BuildCommand == "" {
		c.Frontend.Enabled = true
	}
	if c.Frontend.BindingsDir == "" {
		c.Frontend.BindingsDir = "frontend/bindings"
	}
	if c.Frontend.BuildDir == "" {
		c.Frontend.BuildDir = "frontend/dist"
	}
	if len(c.Frontend.Channels) == 0 {
		c.Frontend.Channels = []string{"stable", "beta"}
	}
	if c.Frontend.BuildCommand == "" {
		c.Frontend.BuildCommand = "npm run build"
	}
	if !c.Frontend.CompatAutoCheck {
		c.Frontend.CompatAutoCheck = true
	}

	if c.Update.CheckInterval == "" {
		c.Update.CheckInterval = "6h"
	}
	if len(c.Update.Channels) == 0 {
		c.Update.Channels = []string{"stable", "beta"}
	}

	if c.Release.Provider == "" {
		c.Release.Provider = "github"
	}
	if c.Release.GitHub.APIBaseURL == "" {
		c.Release.GitHub.APIBaseURL = "https://api.github.com"
	}
	if c.Release.HTTP.ManifestPath == "" {
		c.Release.HTTP.ManifestPath = "/manifest.json"
	}
	if c.Release.HTTP.DeltaManifestPath == "" {
		c.Release.HTTP.DeltaManifestPath = "/delta/manifest.json"
	}
	if c.Release.HTTP.DownloadPathPrefix == "" {
		c.Release.HTTP.DownloadPathPrefix = "/download"
	}

	if c.Output.Dir == "" {
		c.Output.Dir = "dist"
	}
	if c.Output.ManifestFile == "" {
		c.Output.ManifestFile = "manifest.json"
	}
	if !c.Output.Clean {
		c.Output.Clean = true
	}

	if c.CI.Provider == "" {
		c.CI.Provider = "github"
	}
	if c.CI.Timeout == "" {
		c.CI.Timeout = "30m"
	}
	if !c.CI.Artifacts.Upload {
		c.CI.Artifacts.Upload = true
	}
	if c.CI.Artifacts.RetentionDays == 0 {
		c.CI.Artifacts.RetentionDays = 90
	}

	for i := range c.Targets {
		target := &c.Targets[i]
		if target.ID == "" && target.OS != "" && target.Arch != "" {
			target.ID = target.OS + "-" + target.Arch
		}
		if target.Build.Workdir == "" {
			target.Build.Workdir = "."
		}
		for j := range target.Artifacts {
			artifact := &target.Artifacts[j]
			includeInManifest := artifact.IncludeInManifest
			if includeInManifest == nil {
				defaultValue := defaultManifestArtifactFormat(artifact.Format)
				artifact.IncludeInManifest = &defaultValue
				includeInManifest = artifact.IncludeInManifest
			}
			if artifact.EnableDelta == nil {
				defaultValue := includeInManifest != nil && *includeInManifest
				artifact.EnableDelta = &defaultValue
			}
		}
	}
}

func defaultManifestArtifactFormat(format string) bool {
	switch format {
	case "app", "dmg", "exe", "nsis", "appimage", "deb", "rpm", "binary", "zip":
		return true
	default:
		return false
	}
}
