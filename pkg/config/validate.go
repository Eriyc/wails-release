package config

import (
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	validVersionSources   = []string{"git", "file"}
	validTargetOS         = []string{"darwin", "windows", "linux"}
	validArch             = []string{"amd64", "arm64"}
	validArtifactSource   = []string{"github-release", "local-cache", "url"}
	validReleaseProviders = []string{"github", "http"}
	validCIProviders      = []string{"github"}
)

func (c *Config) Validate() []ValidationError {
	var errs []ValidationError

	if c.Schema != 2 {
		errs = append(errs, ValidationError{Field: "schema", Message: "must be 2", Fatal: true})
	}

	if strings.TrimSpace(c.App.Name) == "" {
		errs = append(errs, ValidationError{Field: "app.name", Message: "must not be empty", Fatal: true})
	}
	if strings.TrimSpace(c.App.Identifier) == "" {
		errs = append(errs, ValidationError{Field: "app.identifier", Message: "must not be empty", Fatal: true})
	}

	if !slices.Contains(validVersionSources, c.Version.Source) {
		errs = append(errs, ValidationError{Field: "version.source", Message: "must be one of git,file", Fatal: true})
	}
	if c.Version.Source == "file" && strings.TrimSpace(c.Version.File) == "" {
		errs = append(errs, ValidationError{Field: "version.file", Message: "must be set when version.source=file", Fatal: true})
	}

	if len(c.Targets) == 0 {
		errs = append(errs, ValidationError{Field: "targets", Message: "at least one target is required", Fatal: true})
	}

	for i, target := range c.Targets {
		prefix := "targets[" + strconv.Itoa(i) + "]"
		if strings.TrimSpace(target.ID) == "" {
			errs = append(errs, ValidationError{Field: prefix + ".id", Message: "must not be empty", Fatal: true})
		}
		if !slices.Contains(validTargetOS, target.OS) {
			errs = append(errs, ValidationError{Field: prefix + ".os", Message: "unsupported target OS", Fatal: true})
		}
		if !slices.Contains(validArch, target.Arch) {
			errs = append(errs, ValidationError{Field: prefix + ".arch", Message: "unsupported architecture " + target.Arch, Fatal: true})
		}
		if len(target.Build.Argv) == 0 {
			errs = append(errs, ValidationError{Field: prefix + ".build.argv", Message: "must not be empty", Fatal: true})
		}
		if len(target.Artifacts) == 0 {
			errs = append(errs, ValidationError{Field: prefix + ".artifacts", Message: "at least one artifact is required", Fatal: true})
		}

		for j, artifact := range target.Artifacts {
			artifactPrefix := prefix + ".artifacts[" + strconv.Itoa(j) + "]"
			if strings.TrimSpace(artifact.Format) == "" {
				errs = append(errs, ValidationError{Field: artifactPrefix + ".format", Message: "must not be empty", Fatal: true})
			}
			pathSet := strings.TrimSpace(artifact.Path) != ""
			globSet := strings.TrimSpace(artifact.Glob) != ""
			switch {
			case pathSet && globSet:
				errs = append(errs, ValidationError{Field: artifactPrefix, Message: "path and glob are mutually exclusive", Fatal: true})
			case !pathSet && !globSet:
				errs = append(errs, ValidationError{Field: artifactPrefix, Message: "either path or glob is required", Fatal: true})
			}
		}
	}

	if c.Delta.Enabled {
		if c.Delta.Algorithm != "bsdiff" {
			errs = append(errs, ValidationError{Field: "delta.algorithm", Message: "only bsdiff is currently supported", Fatal: true})
		}
		if c.Delta.FromVersions < 0 {
			errs = append(errs, ValidationError{Field: "delta.from_versions", Message: "must be zero or greater", Fatal: true})
		}
		if !slices.Contains(validArtifactSource, c.Delta.OldArtifacts.Source) {
			errs = append(errs, ValidationError{Field: "delta.old_artifacts.source", Message: "unsupported old artifact source", Fatal: true})
		}
		if c.Delta.OldArtifacts.Source == "url" {
			manifestURL := strings.TrimSpace(c.Delta.OldArtifacts.ManifestURL)
			if manifestURL == "" {
				manifestURL = strings.TrimSpace(c.Update.ManifestURL)
			}
			if manifestURL == "" && strings.TrimSpace(c.Release.Provider) == "http" {
				manifestURL = strings.TrimSpace(c.Release.HTTP.BaseURL)
			}
			if manifestURL == "" {
				errs = append(errs, ValidationError{Field: "delta.old_artifacts.manifest_url", Message: "must be set directly or derivable when source=url", Fatal: true})
			}
		}
	}

	if c.Frontend.Enabled {
		if c.Frontend.CompatVersion <= 0 {
			errs = append(errs, ValidationError{Field: "frontend.compat_version", Message: "must be greater than zero", Fatal: true})
		}
		if c.Frontend.CompatAutoCheck && strings.TrimSpace(c.Frontend.BindingsDir) == "" {
			errs = append(errs, ValidationError{Field: "frontend.bindings_dir", Message: "must not be empty", Fatal: true})
		}
		if strings.TrimSpace(c.Frontend.BuildDir) == "" {
			errs = append(errs, ValidationError{Field: "frontend.build_dir", Message: "must not be empty", Fatal: true})
		}
		if strings.TrimSpace(c.Frontend.BuildCommand) == "" {
			errs = append(errs, ValidationError{Field: "frontend.build_command", Message: "must not be empty", Fatal: true})
		}
		if len(c.Frontend.Channels) == 0 {
			errs = append(errs, ValidationError{Field: "frontend.channels", Message: "at least one channel is required", Fatal: true})
		}
	}

	if c.Update.ManifestURL != "" {
		if _, err := url.ParseRequestURI(c.Update.ManifestURL); err != nil {
			errs = append(errs, ValidationError{Field: "update.manifest_url", Message: "must be a valid URL", Fatal: true})
		}
	}
	if c.Update.CheckInterval != "" {
		if _, err := time.ParseDuration(c.Update.CheckInterval); err != nil {
			errs = append(errs, ValidationError{Field: "update.check_interval", Message: "must be a valid duration", Fatal: true})
		}
	}

	if !slices.Contains(validReleaseProviders, c.Release.Provider) {
		errs = append(errs, ValidationError{Field: "release.provider", Message: "must be one of github,http", Fatal: true})
	}
	if strings.TrimSpace(c.Release.GitHub.APIBaseURL) != "" {
		if _, err := url.ParseRequestURI(c.Release.GitHub.APIBaseURL); err != nil {
			errs = append(errs, ValidationError{Field: "release.github.api_base_url", Message: "must be a valid URL", Fatal: true})
		}
	}
	if c.Release.Provider == "http" {
		if strings.TrimSpace(c.Release.HTTP.BaseURL) == "" {
			errs = append(errs, ValidationError{Field: "release.http.base_url", Message: "must be set when release.provider=http", Fatal: true})
		} else if _, err := url.ParseRequestURI(c.Release.HTTP.BaseURL); err != nil {
			errs = append(errs, ValidationError{Field: "release.http.base_url", Message: "must be a valid URL", Fatal: true})
		}
	}

	if strings.TrimSpace(c.Output.Dir) == "" {
		errs = append(errs, ValidationError{Field: "output.dir", Message: "must not be empty", Fatal: true})
	}
	if strings.TrimSpace(filepath.Base(c.Output.ManifestFile)) == "" {
		errs = append(errs, ValidationError{Field: "output.manifest_file", Message: "must not be empty", Fatal: true})
	}

	if !slices.Contains(validCIProviders, c.CI.Provider) {
		errs = append(errs, ValidationError{Field: "ci.provider", Message: "unsupported CI provider", Fatal: true})
	}
	if c.CI.Timeout != "" {
		if _, err := time.ParseDuration(c.CI.Timeout); err != nil {
			errs = append(errs, ValidationError{Field: "ci.timeout", Message: "must be a valid duration", Fatal: true})
		}
	}
	if c.CI.Artifacts.RetentionDays < 0 {
		errs = append(errs, ValidationError{Field: "ci.artifacts.retention_days", Message: "must be zero or greater", Fatal: true})
	}

	return errs
}
