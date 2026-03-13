package build

import (
	"context"
	"log/slog"
	"time"

	internalexec "github.com/you/wailsrel/internal/exec"
	"github.com/you/wailsrel/pkg/config"
)

type Target struct {
	OS            string
	Arch          string
	OutputFormats []string
}

type Artifact struct {
	Path     string // relative to output dir
	OS       string
	Arch     string
	Format   string
	Checksum string
	Size     int64
	Metadata map[string]string
}

type BuildResult struct {
	Artifacts []Artifact
	CompatID  string
	Duration  time.Duration
}

type Builder interface {
	Build(ctx context.Context, target Target) (*BuildResult, error)
	Available(ctx context.Context, target Target) error
}

type Runner interface {
	Run(ctx context.Context, name string, args []string, opts internalexec.Options) (*internalexec.Result, error)
}

type Options struct {
	ProjectDir string
	OutputDir  string
	AppName    string
	Installers config.InstallerConfig
	Timeout    time.Duration
	Logger     *slog.Logger
	Runner     Runner
	TemplateFS interface {
		ReadFile(name string) ([]byte, error)
	}
}

type WailsBuilder struct {
	projectDir string
	outputDir  string
	appName    string
	installers config.InstallerConfig
	timeout    time.Duration
	logger     *slog.Logger
	runner     Runner
	templateFS interface {
		ReadFile(name string) ([]byte, error)
	}
}

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
		installers: opts.Installers,
		timeout:    opts.Timeout,
		logger:     logger,
		runner:     runner,
		templateFS: opts.TemplateFS,
	}
}

type defaultRunner struct{}

func (defaultRunner) Run(ctx context.Context, name string, args []string, opts internalexec.Options) (*internalexec.Result, error) {
	return internalexec.Run(ctx, name, args, opts)
}
