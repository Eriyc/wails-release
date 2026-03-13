package build

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	internalexec "github.com/Eriyc/wailsrel/internal/exec"
	"github.com/Eriyc/wailsrel/pkg/config"
)

type Target struct {
	ID        string
	OS        string
	Arch      string
	Build     config.BuildHookConfig
	Artifacts []config.ArtifactSpec
}

type Artifact struct {
	Path              string
	OS                string
	Arch              string
	Format            string
	Checksum          string
	Size              int64
	Metadata          map[string]string
	PublishName       string
	IncludeInManifest bool
	EnableDelta       bool
}

type BuildResult struct {
	Artifacts []Artifact
	Duration  time.Duration
}

type Builder interface {
	Build(context.Context, Target) (*BuildResult, error)
	Available(context.Context, Target) error
}

type Runner interface {
	Run(ctx context.Context, name string, args []string, opts internalexec.Options) (*internalexec.Result, error)
}

type Options struct {
	ProjectDir string
	OutputDir  string
	AppName    string
	Version    string
	Tag        string
	Timeout    time.Duration
	Logger     *slog.Logger
	Runner     Runner
}

type WailsBuilder struct {
	projectDir string
	outputDir  string
	appName    string
	version    string
	tag        string
	timeout    time.Duration
	logger     *slog.Logger
	runner     Runner
}

type defaultRunner struct{}

func NewBuilder(opts Options) *WailsBuilder {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	runner := opts.Runner
	if runner == nil {
		runner = defaultRunner{}
	}

	return &WailsBuilder{
		projectDir: opts.ProjectDir,
		outputDir:  opts.OutputDir,
		appName:    opts.AppName,
		version:    opts.Version,
		tag:        opts.Tag,
		timeout:    opts.Timeout,
		logger:     logger,
		runner:     runner,
	}
}

func (defaultRunner) Run(ctx context.Context, name string, args []string, opts internalexec.Options) (*internalexec.Result, error) {
	return internalexec.Run(ctx, name, args, opts)
}

func RequiredTools(target Target) []string {
	tools := make([]string, 0, len(target.Build.Requires)+1)
	if len(target.Build.Argv) > 0 && strings.TrimSpace(target.Build.Argv[0]) != "" {
		tools = append(tools, strings.TrimSpace(target.Build.Argv[0]))
	}
	tools = append(tools, target.Build.Requires...)
	return uniqueStrings(tools)
}

func (b *WailsBuilder) Available(_ context.Context, target Target) error {
	var missing []string
	for _, tool := range RequiredTools(target) {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required tools for %s: %v", target.ID, missing)
	}
	return nil
}

func (b *WailsBuilder) Build(ctx context.Context, target Target) (*BuildResult, error) {
	start := time.Now()

	if err := os.MkdirAll(b.outputDir, 0o755); err != nil {
		return nil, err
	}

	vars := b.templateVars(target)
	workdir := resolveDir(b.projectDir, applyTemplate(target.Build.Workdir, vars))
	argv := applyTemplates(target.Build.Argv, vars)
	if len(argv) == 0 {
		return nil, fmt.Errorf("target %s build argv is empty", target.ID)
	}
	env := renderEnv(target.Build.Env, vars)

	if _, err := b.runner.Run(ctx, argv[0], argv[1:], internalexec.Options{
		Dir:     workdir,
		Env:     env,
		Timeout: b.timeout,
		Logger:  b.logger,
	}); err != nil {
		return nil, fmt.Errorf("%s %v: %w", argv[0], argv[1:], err)
	}

	artifacts, err := b.stageArtifacts(target, vars)
	if err != nil {
		return nil, err
	}

	return &BuildResult{
		Artifacts: artifacts,
		Duration:  time.Since(start),
	}, nil
}

func (b *WailsBuilder) stageArtifacts(target Target, vars map[string]string) ([]Artifact, error) {
	artifacts := make([]Artifact, 0, len(target.Artifacts))
	for _, spec := range target.Artifacts {
		sourcePath, err := discoverArtifactPath(b.projectDir, spec, vars)
		if err != nil {
			return nil, fmt.Errorf("target %s artifact %s: %w", target.ID, spec.Format, err)
		}
		staged, err := b.stageArtifact(target, spec, sourcePath, vars)
		if err != nil {
			return nil, fmt.Errorf("stage %s: %w", sourcePath, err)
		}
		artifacts = append(artifacts, staged)
	}

	slices.SortFunc(artifacts, func(a, b Artifact) int {
		return strings.Compare(a.Path, b.Path)
	})

	return artifacts, nil
}

func (b *WailsBuilder) stageArtifact(target Target, spec config.ArtifactSpec, sourcePath string, vars map[string]string) (Artifact, error) {
	relative := filepath.Join(target.OS, target.Arch, filepath.Base(sourcePath))
	destination := filepath.Join(b.outputDir, relative)

	if err := os.RemoveAll(destination); err != nil {
		return Artifact{}, err
	}
	if err := copyPath(sourcePath, destination); err != nil {
		return Artifact{}, err
	}

	checksum, err := ComputeChecksum(destination)
	if err != nil {
		return Artifact{}, err
	}
	if err := writeChecksumFile(destination, checksum); err != nil {
		return Artifact{}, err
	}
	size, err := pathSize(destination)
	if err != nil {
		return Artifact{}, err
	}

	return Artifact{
		Path:              normalizeArtifactPath(relative),
		OS:                target.OS,
		Arch:              target.Arch,
		Format:            spec.Format,
		Checksum:          "sha256:" + checksum,
		Size:              size,
		Metadata:          map[string]string{},
		PublishName:       strings.TrimSpace(applyTemplate(spec.PublishName, vars)),
		IncludeInManifest: spec.IncludeInManifest != nil && *spec.IncludeInManifest,
		EnableDelta:       spec.EnableDelta != nil && *spec.EnableDelta,
	}, nil
}

func discoverArtifactPath(projectDir string, spec config.ArtifactSpec, vars map[string]string) (string, error) {
	if path := strings.TrimSpace(spec.Path); path != "" {
		resolved := resolveDir(projectDir, applyTemplate(path, vars))
		if err := requirePath(resolved); err != nil {
			return "", err
		}
		return resolved, nil
	}

	pattern := strings.TrimSpace(spec.Glob)
	if pattern == "" {
		return "", fmt.Errorf("path or glob is required")
	}

	matches, err := filepath.Glob(resolveDir(projectDir, applyTemplate(pattern, vars)))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("glob matched no files")
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("glob matched multiple files: %v", matches)
	}
	if err := requirePath(matches[0]); err != nil {
		return "", err
	}
	return matches[0], nil
}

func applyTemplates(values []string, vars map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, applyTemplate(value, vars))
	}
	return out
}

func renderEnv(values map[string]string, vars map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+applyTemplate(values[key], vars))
	}
	return out
}

func applyTemplate(value string, vars map[string]string) string {
	replacements := make([]string, 0, len(vars)*2)
	for key, rendered := range vars {
		replacements = append(replacements, "{{"+key+"}}", rendered)
	}
	replacer := strings.NewReplacer(replacements...)
	return replacer.Replace(value)
}

func resolveDir(root, value string) string {
	if value == "" {
		return root
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(root, value))
}

func (b *WailsBuilder) templateVars(target Target) map[string]string {
	return map[string]string{
		"project_dir": b.projectDir,
		"output_dir":  b.outputDir,
		"app_name":    b.appName,
		"version":     b.version,
		"tag":         b.tag,
		"os":          target.OS,
		"arch":        target.Arch,
		"target_id":   target.ID,
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
