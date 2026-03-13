package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultFileName = "wailsrel.yaml"

type Config struct {
	Schema   int            `yaml:"schema"`
	App      AppConfig      `yaml:"app"`
	Version  VersionConfig  `yaml:"version"`
	Targets  []TargetConfig `yaml:"targets"`
	Delta    DeltaConfig    `yaml:"delta"`
	Frontend FrontendConfig `yaml:"frontend"`
	Update   UpdateConfig   `yaml:"update"`
	Release  ReleaseConfig  `yaml:"release"`
	Output   OutputConfig   `yaml:"output"`
	CI       CIConfig       `yaml:"ci"`
}

type AppConfig struct {
	Name        string `yaml:"name"`
	Identifier  string `yaml:"identifier"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
	URL         string `yaml:"url"`
}

type VersionConfig struct {
	Source           string `yaml:"source"`
	File             string `yaml:"file"`
	TagPrefix        string `yaml:"tag_prefix"`
	PrereleaseFormat string `yaml:"prerelease_format"`
}

type TargetConfig struct {
	ID        string          `yaml:"id"`
	OS        string          `yaml:"os"`
	Arch      string          `yaml:"arch"`
	Build     BuildHookConfig `yaml:"build"`
	Artifacts []ArtifactSpec  `yaml:"artifacts"`
}

type BuildHookConfig struct {
	Argv     []string          `yaml:"argv"`
	Workdir  string            `yaml:"workdir"`
	Env      map[string]string `yaml:"env"`
	Requires []string          `yaml:"requires"`
}

type ArtifactSpec struct {
	Format            string `yaml:"format"`
	Path              string `yaml:"path"`
	Glob              string `yaml:"glob"`
	PublishName       string `yaml:"publish_name"`
	IncludeInManifest *bool  `yaml:"include_in_manifest"`
	EnableDelta       *bool  `yaml:"enable_delta"`
}

type DeltaConfig struct {
	Enabled      bool               `yaml:"enabled"`
	Algorithm    string             `yaml:"algorithm"`
	FromVersions int                `yaml:"from_versions"`
	OldArtifacts OldArtifactsConfig `yaml:"old_artifacts"`
}

type OldArtifactsConfig struct {
	Source       string `yaml:"source"`
	Repository   string `yaml:"repository"`
	CacheDir     string `yaml:"cache_dir"`
	ManifestURL  string `yaml:"manifest_url"`
	AuthTokenEnv string `yaml:"auth_token_env"`
}

type FrontendConfig struct {
	Enabled         bool     `yaml:"enabled"`
	CompatVersion   int      `yaml:"compat_version"`
	CompatAutoCheck bool     `yaml:"compat_auto_check"`
	BindingsDir     string   `yaml:"bindings_dir"`
	BuildDir        string   `yaml:"build_dir"`
	Channels        []string `yaml:"channels"`
	BuildCommand    string   `yaml:"build_command"`
}

type UpdateConfig struct {
	ManifestURL    string   `yaml:"manifest_url"`
	CheckInterval  string   `yaml:"check_interval"`
	Channels       []string `yaml:"channels"`
	AllowDowngrade bool     `yaml:"allow_downgrade"`
	MandatoryMin   string   `yaml:"mandatory_min"`
}

type ReleaseConfig struct {
	Provider string              `yaml:"provider"`
	GitHub   ReleaseGitHubConfig `yaml:"github"`
	HTTP     ReleaseHTTPConfig   `yaml:"http"`
}

type ReleaseGitHubConfig struct {
	Repository string `yaml:"repository"`
	APIBaseURL string `yaml:"api_base_url"`
}

type ReleaseHTTPConfig struct {
	BaseURL            string `yaml:"base_url"`
	ManifestPath       string `yaml:"manifest_path"`
	DeltaManifestPath  string `yaml:"delta_manifest_path"`
	DownloadPathPrefix string `yaml:"download_path_prefix"`
}

type OutputConfig struct {
	Dir          string `yaml:"dir"`
	ManifestFile string `yaml:"manifest_file"`
	Clean        bool   `yaml:"clean"`
}

type CIConfig struct {
	Provider  string            `yaml:"provider"`
	Timeout   string            `yaml:"timeout"`
	Artifacts CIArtifactsConfig `yaml:"artifacts"`
}

type CIArtifactsConfig struct {
	Upload        bool `yaml:"upload"`
	RetentionDays int  `yaml:"retention_days"`
}

type ValidationError struct {
	Field   string
	Message string
	Fatal   bool
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}

	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func Load(dir string) (*Config, error) {
	path, err := resolvePath(dir)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if errs := cfg.Validate(); hasFatal(errs) {
		return nil, errors.Join(validationErrors(errs)...)
	}

	return cfg, nil
}

func validationErrors(errs []ValidationError) []error {
	out := make([]error, 0, len(errs))
	for _, err := range errs {
		if err.Fatal {
			out = append(out, err)
		}
	}

	return out
}

func hasFatal(errs []ValidationError) bool {
	for _, err := range errs {
		if err.Fatal {
			return true
		}
	}

	return false
}

func resolvePath(path string) (string, error) {
	if path == "" {
		path = "."
	}

	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return filepath.Join(path, DefaultFileName), nil
	}
	if err == nil {
		return path, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		return path, nil
	}

	return filepath.Join(path, DefaultFileName), nil
}
