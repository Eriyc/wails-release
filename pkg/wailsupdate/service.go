package wailsupdate

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/Eriyc/wailsrel/pkg/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultEventPrefix  = "update"
	defaultCheckTimeout = 30 * time.Second
	defaultApplyTimeout = 10 * time.Minute
)

type Options struct {
	ManifestURL       string
	CurrentVersion    string
	Channel           string
	NativeCompat      string
	TargetPath        string
	TempDir           string
	Client            *http.Client
	Checker           update.Checker
	Applier           update.Applier
	EventPrefix       string
	CheckTimeout      time.Duration
	ApplyTimeout      time.Duration
	AutoCheckInterval time.Duration
	DescribeState     func(*State)
	Relaunch          func(context.Context, RelaunchRequest) error
}

type State struct {
	ManifestURL     string            `json:"manifestURL"`
	CurrentVersion  string            `json:"currentVersion"`
	CurrentHash     string            `json:"currentHash,omitempty"`
	Channel         string            `json:"channel"`
	NativeCompat    string            `json:"nativeCompat,omitempty"`
	TargetPath      string            `json:"targetPath"`
	TempDir         string            `json:"tempDir"`
	LastCheckedAt   string            `json:"lastCheckedAt,omitempty"`
	PendingRestart  bool              `json:"pendingRestart"`
	AvailableUpdate *UpdateView       `json:"availableUpdate,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Notes           []string          `json:"notes,omitempty"`
	LastError       string            `json:"lastError,omitempty"`
}

type UpdateView struct {
	Version       string `json:"version"`
	Channel       string `json:"channel"`
	ReleaseNotes  string `json:"releaseNotes,omitempty"`
	Mandatory     bool   `json:"mandatory"`
	ArtifactURL   string `json:"artifactURL"`
	ArtifactHash  string `json:"artifactHash"`
	ArtifactSize  int64  `json:"artifactSize"`
	DeltaURL      string `json:"deltaURL,omitempty"`
	DeltaHash     string `json:"deltaHash,omitempty"`
	DeltaSize     int64  `json:"deltaSize,omitempty"`
	DeltaFromHash string `json:"deltaFromHash,omitempty"`
}

type CheckResponse struct {
	CheckedAt string      `json:"checkedAt"`
	Available bool        `json:"available"`
	Update    *UpdateView `json:"update,omitempty"`
	Error     string      `json:"error,omitempty"`
}

type ActionResponse struct {
	StartedAt string `json:"startedAt"`
	Applied   bool   `json:"applied"`
	Restarted bool   `json:"restarted"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

type LogEvent struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	At      string `json:"at"`
}

type ProgressEvent struct {
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"`
}

type RelaunchRequest struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
}

type Service struct {
	opts    Options
	manager *update.Manager

	opMu sync.Mutex
	mu   sync.Mutex

	available      *update.UpdateInfo
	lastCheckedAt  string
	pendingRestart bool
	lastError      string

	autoCheckCancel context.CancelFunc
	autoCheckWG     sync.WaitGroup
}

var (
	registerEventNames sync.Map
	quitApplication    = func() {
		if app := application.Get(); app != nil {
			app.Quit()
		}
	}
)

func NewService(opts Options) *Service {
	opts = normalizeOptions(opts)

	service := &Service{opts: opts}
	service.manager = update.NewManager(update.ManagerOpts{
		Checker: opts.Checker,
		Applier: opts.Applier,
		OnProgress: func(downloaded, total int64) {
			service.emitProgress(downloaded, total)
		},
		OnRestart: func() error {
			service.markPendingRestart()
			return nil
		},
	})
	return service
}

func RegisterEvents(prefix string) {
	prefix = normalizePrefix(prefix)
	registerEventOnce(prefix+":state", func() {
		application.RegisterEvent[State](prefix + ":state")
	})
	registerEventOnce(prefix+":progress", func() {
		application.RegisterEvent[ProgressEvent](prefix + ":progress")
	})
	registerEventOnce(prefix+":log", func() {
		application.RegisterEvent[LogEvent](prefix + ":log")
	})
}

func (s *Service) GetState() State {
	return s.snapshotState()
}

func (s *Service) CheckNow() CheckResponse {
	response, _ := s.check(context.Background())
	return response
}

func (s *Service) ApplyPending() ActionResponse {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	response := ActionResponse{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   "No update has been applied.",
	}

	if err := s.validate(); err != nil {
		response.Error = err.Error()
		response.Message = "Configuration is incomplete."
		s.setLastError(err)
		s.emitLog("error", err.Error())
		s.emitState()
		return response
	}
	if err := ensureSupportedPlatform(); err != nil {
		response.Error = err.Error()
		response.Message = "Update apply failed."
		s.setLastError(err)
		s.emitLog("error", err.Error())
		s.emitState()
		return response
	}
	if s.isPendingRestart() {
		response.Message = "An update has already been staged. Restart the application to launch it."
		return response
	}

	info := s.cloneAvailable()
	if info == nil {
		check, current := s.checkLocked(context.Background())
		if current != nil {
			return ActionResponse{
				StartedAt: response.StartedAt,
				Message:   "Unable to load an update before applying.",
				Error:     check.Error,
			}
		}
		info = s.cloneAvailable()
	}
	if info == nil {
		response.Message = "There is no available update to apply."
		s.emitLog("info", response.Message)
		s.emitState()
		return response
	}

	s.emitLog("info", fmt.Sprintf("Downloading %s from %s", info.Version, info.ArtifactURL))

	ctx, cancel := context.WithTimeout(context.Background(), s.opts.ApplyTimeout)
	defer cancel()

	if err := s.manager.Apply(ctx, info); err != nil {
		response.Error = err.Error()
		response.Message = "Update apply failed."
		s.setLastError(err)
		s.emitLog("error", err.Error())
		s.emitState()
		return response
	}

	response.Applied = true
	response.Message = "Update staged successfully. Restart the application to launch it."
	s.clearAvailable()
	s.clearLastError()
	s.emitLog("info", response.Message)
	s.emitState()
	return response
}

func (s *Service) Restart() ActionResponse {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	response := ActionResponse{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   "No restart is pending.",
	}

	if !s.isPendingRestart() {
		return response
	}
	if err := ensureSupportedPlatform(); err != nil {
		response.Error = err.Error()
		response.Message = "Restart failed."
		s.setLastError(err)
		s.emitLog("error", err.Error())
		s.emitState()
		return response
	}

	request := RelaunchRequest{
		Executable: firstNonEmpty(DefaultTargetPath(), s.opts.TargetPath),
		Args:       append([]string(nil), os.Args[1:]...),
	}
	if err := s.opts.Relaunch(context.Background(), request); err != nil {
		response.Error = err.Error()
		response.Message = "Restart failed."
		s.setLastError(err)
		s.emitLog("error", err.Error())
		s.emitState()
		return response
	}

	response.Restarted = true
	response.Message = "Restarting application."
	s.clearPendingRestart()
	s.clearLastError()
	s.emitLog("info", response.Message)
	s.emitState()
	quitApplication()
	return response
}

func (s *Service) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	if s.opts.AutoCheckInterval <= 0 {
		return nil
	}

	autoCheckCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.autoCheckCancel = cancel
	s.mu.Unlock()

	s.autoCheckWG.Add(1)
	go func() {
		defer s.autoCheckWG.Done()
		ticker := time.NewTicker(s.opts.AutoCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-autoCheckCtx.Done():
				return
			case <-ticker.C:
				s.check(autoCheckCtx)
			}
		}
	}()

	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	cancel := s.autoCheckCancel
	s.autoCheckCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.autoCheckWG.Wait()
	return nil
}

func DefaultTargetPath() string {
	targetPath, err := os.Executable()
	if err != nil {
		return ""
	}
	return targetPath
}

func CurrentExecutableHash(targetPath string) (string, error) {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return "", fmt.Errorf("target path is empty")
	}
	checksum, err := build.ComputeChecksum(targetPath)
	if err != nil {
		return "", err
	}
	return "sha256:" + checksum, nil
}

func GitHubLatestManifestURL(repository string) string {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	if repository == "" {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/releases/latest/download/manifest.json", repository)
}

func BearerTransport(base http.RoundTripper, token string) http.RoundTripper {
	return bearerTransport{
		base:  base,
		token: strings.TrimSpace(token),
	}
}

func normalizeOptions(opts Options) Options {
	opts.ManifestURL = strings.TrimSpace(opts.ManifestURL)
	opts.CurrentVersion = strings.TrimSpace(opts.CurrentVersion)
	opts.Channel = strings.TrimSpace(opts.Channel)
	opts.NativeCompat = strings.TrimSpace(opts.NativeCompat)
	opts.TargetPath = strings.TrimSpace(opts.TargetPath)
	opts.TempDir = strings.TrimSpace(opts.TempDir)
	opts.EventPrefix = normalizePrefix(opts.EventPrefix)
	if opts.TargetPath == "" {
		opts.TargetPath = DefaultTargetPath()
	}
	if opts.TempDir == "" {
		opts.TempDir = filepath.Clean(os.TempDir())
	}
	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}
	if opts.CheckTimeout <= 0 {
		opts.CheckTimeout = defaultCheckTimeout
	}
	if opts.ApplyTimeout <= 0 {
		opts.ApplyTimeout = defaultApplyTimeout
	}
	if opts.Checker == nil {
		opts.Checker = update.NewChecker(opts.Client)
	}
	if opts.Applier == nil {
		opts.Applier = update.NewApplier(update.ApplierOptions{
			Client:     opts.Client,
			TargetPath: opts.TargetPath,
			TempDir:    opts.TempDir,
		})
	}
	if opts.Relaunch == nil {
		opts.Relaunch = defaultRelaunch
	}
	return opts
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return defaultEventPrefix
	}
	return prefix
}

func registerEventOnce(name string, register func()) {
	if _, loaded := registerEventNames.LoadOrStore(name, struct{}{}); loaded {
		return
	}
	register()
}

func (s *Service) check(parent context.Context) (CheckResponse, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.checkLocked(parent)
}

func (s *Service) checkLocked(parent context.Context) (CheckResponse, error) {
	response := CheckResponse{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	s.setLastCheckedAt(response.CheckedAt)

	if err := s.validate(); err != nil {
		response.Error = err.Error()
		s.setLastError(err)
		s.emitLog("error", err.Error())
		s.emitState()
		return response, err
	}

	currentHash, hashErr := CurrentExecutableHash(s.opts.TargetPath)
	if hashErr != nil {
		s.emitLog("warn", "Continuing without a current executable checksum: "+hashErr.Error())
		currentHash = ""
	}

	ctx, cancel := context.WithTimeout(parent, s.opts.CheckTimeout)
	defer cancel()

	s.manager.CheckOpts = update.CheckOpts{
		CurrentVersion: s.opts.CurrentVersion,
		CurrentHash:    currentHash,
		NativeCompat:   s.opts.NativeCompat,
		Channel:        s.opts.Channel,
		ManifestURL:    s.opts.ManifestURL,
	}

	result, err := s.manager.CheckNow(ctx)
	if err != nil {
		response.Error = err.Error()
		s.setLastError(err)
		s.emitLog("error", err.Error())
		s.emitState()
		return response, err
	}

	if result == nil || !result.Available || result.Native == nil {
		s.clearAvailable()
		s.clearLastError()
		s.emitLog("info", "No newer release was found at the configured manifest URL.")
		s.emitState()
		return response, nil
	}

	response.Available = true
	response.Update = toUpdateView(result.Native)
	s.setAvailable(result.Native)
	s.clearLastError()
	s.emitLog("info", fmt.Sprintf("Update %s is available.", result.Native.Version))
	s.emitState()
	return response, nil
}

func (s *Service) snapshotState() State {
	state := State{
		ManifestURL:    s.opts.ManifestURL,
		CurrentVersion: s.opts.CurrentVersion,
		Channel:        s.opts.Channel,
		NativeCompat:   s.opts.NativeCompat,
		TargetPath:     s.opts.TargetPath,
		TempDir:        s.opts.TempDir,
	}

	if currentHash, err := CurrentExecutableHash(s.opts.TargetPath); err == nil {
		state.CurrentHash = currentHash
	} else if strings.TrimSpace(s.opts.TargetPath) != "" {
		state.Notes = append(state.Notes, "Current executable checksum is unavailable: "+err.Error())
	}

	s.mu.Lock()
	state.LastCheckedAt = s.lastCheckedAt
	state.PendingRestart = s.pendingRestart
	state.AvailableUpdate = toUpdateView(s.available)
	state.LastError = s.lastError
	s.mu.Unlock()

	if s.opts.DescribeState != nil {
		s.opts.DescribeState(&state)
	}
	if len(state.Metadata) == 0 {
		state.Metadata = nil
	}
	if len(state.Notes) == 0 {
		state.Notes = nil
	}
	return state
}

func (s *Service) emitState() {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit(s.opts.EventPrefix+":state", s.snapshotState())
}

func (s *Service) emitProgress(downloaded, total int64) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit(s.opts.EventPrefix+":progress", ProgressEvent{
		Downloaded: downloaded,
		Total:      total,
	})
}

func (s *Service) emitLog(level, message string) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit(s.opts.EventPrefix+":log", LogEvent{
		Level:   level,
		Message: message,
		At:      time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Service) validate() error {
	if s.opts.ManifestURL == "" {
		return fmt.Errorf("manifest URL is required")
	}
	if s.opts.CurrentVersion == "" {
		return fmt.Errorf("current version is required")
	}
	if s.opts.TargetPath == "" {
		return fmt.Errorf("target path is required")
	}
	return nil
}

func (s *Service) setAvailable(info *update.UpdateInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.available = cloneUpdateInfo(info)
}

func (s *Service) clearAvailable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.available = nil
}

func (s *Service) cloneAvailable() *update.UpdateInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneUpdateInfo(s.available)
}

func (s *Service) markPendingRestart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingRestart = true
}

func (s *Service) clearPendingRestart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingRestart = false
}

func (s *Service) isPendingRestart() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingRestart
}

func (s *Service) setLastCheckedAt(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCheckedAt = value
}

func (s *Service) setLastError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = err.Error()
}

func (s *Service) clearLastError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = ""
}

func ensureSupportedPlatform() error {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		return nil
	default:
		return fmt.Errorf("native self-update is not supported on %s", runtime.GOOS)
	}
}

func defaultRelaunch(ctx context.Context, request RelaunchRequest) error {
	_ = ctx
	if err := ensureSupportedPlatform(); err != nil {
		return err
	}
	if strings.TrimSpace(request.Executable) == "" {
		return fmt.Errorf("relaunch executable is required")
	}

	cmd := exec.Command(request.Executable, request.Args...)
	cmd.Env = os.Environ()
	return cmd.Start()
}

func toUpdateView(info *update.UpdateInfo) *UpdateView {
	if info == nil {
		return nil
	}
	return &UpdateView{
		Version:       info.Version,
		Channel:       info.Channel,
		ReleaseNotes:  info.ReleaseNotes,
		Mandatory:     info.Mandatory,
		ArtifactURL:   info.ArtifactURL,
		ArtifactHash:  info.ArtifactHash,
		ArtifactSize:  info.ArtifactSize,
		DeltaURL:      info.DeltaURL,
		DeltaHash:     info.DeltaHash,
		DeltaSize:     info.DeltaSize,
		DeltaFromHash: info.DeltaFromHash,
	}
}

func cloneUpdateInfo(info *update.UpdateInfo) *update.UpdateInfo {
	if info == nil {
		return nil
	}
	cloned := *info
	if info.Frontend != nil {
		frontend := *info.Frontend
		cloned.Frontend = &frontend
	}
	return &cloned
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	clone := req.Clone(req.Context())
	if t.token != "" {
		clone.Header.Set("Authorization", "Bearer "+t.token)
	}
	return base.RoundTrip(clone)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
