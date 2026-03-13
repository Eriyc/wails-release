package sign

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	internalexec "github.com/Eriyc/wailsrel/internal/exec"
	"github.com/Eriyc/wailsrel/pkg/config"
)

type SignOpts struct {
	Identity    string
	Notarize    bool
	Credentials map[string]string
	Timeout     time.Duration
}

type SignResult struct {
	Signed    bool
	Notarized bool
	Duration  time.Duration
}

type Signer interface {
	Sign(ctx context.Context, path string, opts SignOpts) (*SignResult, error)
	Verify(ctx context.Context, path string) error
	Available(ctx context.Context) error
	Provider() string
}

type Runner interface {
	Run(ctx context.Context, name string, args []string, opts internalexec.Options) (*internalexec.Result, error)
}

type Options struct {
	Logger *slog.Logger
	Runner Runner
}

type defaultRunner struct{}

func (defaultRunner) Run(ctx context.Context, name string, args []string, opts internalexec.Options) (*internalexec.Result, error) {
	return internalexec.Run(ctx, name, args, opts)
}

type baseSigner struct {
	cfg    config.SignConfig
	logger *slog.Logger
	runner Runner
}

func NewSigner(cfg config.SignConfig) (Signer, error) {
	return NewSignerWithOptions(cfg, Options{})
}

func NewSignerWithOptions(cfg config.SignConfig, opts Options) (Signer, error) {
	provider := strings.TrimSpace(cfg.Provider)
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	runner := opts.Runner
	if runner == nil {
		runner = defaultRunner{}
	}

	base := baseSigner{
		cfg:    cfg,
		logger: logger,
		runner: runner,
	}

	switch provider {
	case "", "none":
		return &noopSigner{baseSigner: base}, nil
	case "apple":
		return &appleSigner{baseSigner: base}, nil
	case "apple-rcodesign":
		return &rcodesignSigner{baseSigner: base}, nil
	case "azure":
		return &azureSigner{baseSigner: base}, nil
	default:
		return nil, fmt.Errorf("unsupported signing provider %q", provider)
	}
}

func RequiredTools(cfg config.SignConfig) []string {
	switch strings.TrimSpace(cfg.Provider) {
	case "", "none":
		return nil
	case "apple":
		tools := []string{"codesign"}
		if cfg.Notarize {
			tools = append(tools, "xcrun")
		}
		return tools
	case "apple-rcodesign":
		return []string{"rcodesign"}
	case "azure":
		return []string{"signtool"}
	default:
		return nil
	}
}

func (s baseSigner) run(ctx context.Context, timeout time.Duration, name string, args ...string) (*internalexec.Result, error) {
	result, err := s.runner.Run(ctx, name, args, internalexec.Options{
		Timeout: timeout,
		Logger:  s.logger,
	})
	if err != nil {
		if result != nil && strings.TrimSpace(result.Stderr) != "" {
			return result, fmt.Errorf("%s %v: %w: %s", name, args, err, strings.TrimSpace(result.Stderr))
		}
		return result, fmt.Errorf("%s %v: %w", name, args, err)
	}

	return result, nil
}

func (s baseSigner) requireTools(names ...string) error {
	var missing []string
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required signing tools: %s", strings.Join(missing, ", "))
	}

	return nil
}

func credentialValue(opts SignOpts, key string, fallback string, envKeys ...string) string {
	if opts.Credentials != nil {
		if value := strings.TrimSpace(opts.Credentials[key]); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(fallback); value != "" {
		return value
	}
	for _, envKey := range envKeys {
		if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
			return value
		}
	}
	return ""
}

func requireValue(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func ensureFileExists(field, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}

func combineErrors(errs ...error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return errors.Join(filtered...)
}
