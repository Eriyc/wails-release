package build

import (
	"context"
	"log/slog"
	"time"

	internalexec "github.com/Eriyc/wailsrel/internal/exec"
	"github.com/Eriyc/wailsrel/pkg/config"
	"github.com/Eriyc/wailsrel/pkg/sign"
)

type Target struct {
	OS            string
	Arch          string
	OutputFormats []string
	Sign          config.SignConfig
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
	NewSigner  func(config.SignConfig) (sign.Signer, error)
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
	newSigner  func(config.SignConfig) (sign.Signer, error)
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

	newSigner := opts.NewSigner
	if newSigner == nil {
		newSigner = sign.NewSigner
	}

	return &WailsBuilder{
		projectDir: opts.ProjectDir,
		outputDir:  opts.OutputDir,
		appName:    opts.AppName,
		installers: opts.Installers,
		timeout:    opts.Timeout,
		logger:     logger,
		runner:     runner,
		newSigner:  newSigner,
		templateFS: opts.TemplateFS,
	}
}

type defaultRunner struct{}

func (defaultRunner) Run(ctx context.Context, name string, args []string, opts internalexec.Options) (*internalexec.Result, error) {
	return internalexec.Run(ctx, name, args, opts)
}
