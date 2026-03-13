package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	osexec "os/exec"
	"time"
)

type Options struct {
	Dir     string
	Env     []string
	Timeout time.Duration
	Logger  *slog.Logger
}

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

func Run(ctx context.Context, name string, args []string, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Debug("running command", "name", name, "args", args, "dir", opts.Dir)

	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Dir = opts.Dir
	if len(opts.Env) > 0 {
		cmd.Env = append(cmd.Environ(), opts.Env...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(err),
		Duration: duration,
	}

	logger.Debug("command finished", "name", name, "exit_code", result.ExitCode, "duration", duration)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result, fmt.Errorf("command timed out after %s: %w", opts.Timeout, ctx.Err())
	}
	if err != nil {
		return result, fmt.Errorf("command failed: %w", err)
	}

	return result, nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	return -1
}
