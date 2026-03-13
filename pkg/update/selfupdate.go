package update

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type RestartFunc func() error

type Manager struct {
	Checker       Checker
	Applier       Applier
	OnRestart     RestartFunc
	OnError       func(error)
	OnAvailable   func(CheckResult)
	OnProgress    ProgressFunc
	AutoCheck     bool
	CheckInterval time.Duration
	CheckOpts     CheckOpts
}

type ManagerOpts struct {
	Checker       Checker
	Applier       Applier
	OnRestart     RestartFunc
	OnError       func(error)
	OnAvailable   func(CheckResult)
	OnProgress    ProgressFunc
	AutoCheck     bool
	CheckInterval time.Duration
	CheckOpts     CheckOpts
}

func NewManager(opts ManagerOpts) *Manager {
	return &Manager{
		Checker:       opts.Checker,
		Applier:       opts.Applier,
		OnRestart:     opts.OnRestart,
		OnError:       opts.OnError,
		OnAvailable:   opts.OnAvailable,
		OnProgress:    opts.OnProgress,
		AutoCheck:     opts.AutoCheck,
		CheckInterval: opts.CheckInterval,
		CheckOpts:     opts.CheckOpts,
	}
}

func (m *Manager) Start(ctx context.Context) {
	if !m.AutoCheck || m.Checker == nil || m.CheckInterval <= 0 {
		return
	}

	ticker := time.NewTicker(m.CheckInterval)
	defer ticker.Stop()

	for {
		_, _ = m.CheckNow(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) CheckNow(ctx context.Context) (*CheckResult, error) {
	if m.Checker == nil {
		return nil, fmt.Errorf("checker is required")
	}
	result, err := m.Checker.Check(ctx, m.CheckOpts)
	if err != nil {
		if m.OnError != nil {
			m.OnError(err)
		}
		return nil, err
	}
	if result != nil && result.Available && m.OnAvailable != nil {
		m.OnAvailable(*result)
	}
	return result, nil
}

func (m *Manager) Apply(ctx context.Context, info *UpdateInfo) error {
	if info == nil {
		return fmt.Errorf("update info is required")
	}
	if m.Applier == nil {
		return fmt.Errorf("applier is required")
	}
	if info.Frontend != nil && strings.TrimSpace(info.ArtifactURL) == "" {
		if err := m.Applier.ApplyFrontend(ctx, info.Frontend, m.OnProgress); err != nil {
			if m.OnError != nil {
				m.OnError(err)
			}
			return err
		}
		return nil
	}
	if err := m.Applier.ApplyNative(ctx, info, m.OnProgress); err != nil {
		if m.OnError != nil {
			m.OnError(err)
		}
		return err
	}
	if m.OnRestart != nil {
		return m.OnRestart()
	}
	return nil
}
