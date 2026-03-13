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
	validVersionSources = []string{"git", "file"}
	validTargetOS       = []string{"darwin", "windows", "linux"}
	validArch           = []string{"amd64", "arm64"}
	validSignProviders  = []string{"", "none", "apple", "apple-rcodesign", "azure"}
	validOutputFormats  = []string{"app", "dmg", "exe", "nsis", "appimage", "deb", "rpm", "binary", "zip"}
	validArtifactSource = []string{"github-release", "local-cache", "url"}
	validCIProviders    = []string{"github"}
)

var validSignProvidersByOS = map[string][]string{
	"darwin":  {"", "none", "apple", "apple-rcodesign"},
	"windows": {"", "none", "azure"},
	"linux":   {"", "none"},
}

func (c *Config) Validate() []ValidationError {
	var errs []ValidationError

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

		if !slices.Contains(validTargetOS, target.OS) {
			errs = append(errs, ValidationError{Field: prefix + ".os", Message: "unsupported target OS", Fatal: true})
		}
		if len(target.Arch) == 0 {
			errs = append(errs, ValidationError{Field: prefix + ".arch", Message: "at least one architecture is required", Fatal: true})
		}
		for _, arch := range target.Arch {
			if !slices.Contains(validArch, arch) {
				errs = append(errs, ValidationError{Field: prefix + ".arch", Message: "unsupported architecture " + arch, Fatal: true})
			}
		}
		if len(target.OutputFormats) == 0 {
			errs = append(errs, ValidationError{Field: prefix + ".output_formats", Message: "at least one output format is required", Fatal: true})
		}
		for _, format := range target.OutputFormats {
			if !slices.Contains(validOutputFormats, format) {
				errs = append(errs, ValidationError{Field: prefix + ".output_formats", Message: "unsupported output format " + format, Fatal: true})
			}
		}
		if !slices.Contains(validSignProviders, target.Sign.Provider) {
			errs = append(errs, ValidationError{Field: prefix + ".sign.provider", Message: "unsupported signing provider", Fatal: true})
		}
		if allowed := validSignProvidersByOS[target.OS]; len(allowed) > 0 && !slices.Contains(allowed, target.Sign.Provider) {
			errs = append(errs, ValidationError{Field: prefix + ".sign.provider", Message: "signing provider is not supported for target OS " + target.OS, Fatal: true})
		}
		switch target.Sign.Provider {
		case "apple":
			if strings.TrimSpace(target.Sign.Identity) == "" {
				errs = append(errs, ValidationError{Field: prefix + ".sign.identity", Message: "must be set when sign.provider=apple", Fatal: true})
			}
			if target.Sign.Notarize {
				if strings.TrimSpace(target.Sign.AppleID) == "" {
					errs = append(errs, ValidationError{Field: prefix + ".sign.apple_id", Message: "must be set when notarization is enabled", Fatal: true})
				}
				if strings.TrimSpace(target.Sign.Password) == "" {
					errs = append(errs, ValidationError{Field: prefix + ".sign.password", Message: "must be set when notarization is enabled", Fatal: true})
				}
				if strings.TrimSpace(target.Sign.TeamID) == "" {
					errs = append(errs, ValidationError{Field: prefix + ".sign.team_id", Message: "must be set when notarization is enabled", Fatal: true})
				}
			}
		case "apple-rcodesign":
			if strings.TrimSpace(target.Sign.Identity) == "" {
				errs = append(errs, ValidationError{Field: prefix + ".sign.identity", Message: "must point to a PKCS#12 bundle or certificate file when sign.provider=apple-rcodesign", Fatal: true})
			}
		case "azure":
			if strings.TrimSpace(target.Sign.Endpoint) == "" {
				errs = append(errs, ValidationError{Field: prefix + ".sign.endpoint", Message: "must be set when sign.provider=azure", Fatal: true})
			}
			if strings.TrimSpace(target.Sign.Account) == "" {
				errs = append(errs, ValidationError{Field: prefix + ".sign.account", Message: "must be set when sign.provider=azure", Fatal: true})
			}
			if strings.TrimSpace(target.Sign.Profile) == "" {
				errs = append(errs, ValidationError{Field: prefix + ".sign.profile", Message: "must be set when sign.provider=azure", Fatal: true})
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
	}

	if c.Frontend.Enabled {
		if c.Frontend.CompatVersion <= 0 {
			errs = append(errs, ValidationError{Field: "frontend.compat_version", Message: "must be greater than zero", Fatal: true})
		}
		if strings.TrimSpace(c.Frontend.BindingsDir) == "" {
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
