package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/Eriyc/wailsrel/pkg/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var appVersion = "0.1.0"

type appConfig struct {
	AppName        string
	SourceLabel    string
	BaseURL        string
	ManifestURL    string
	CurrentVersion string
	Channel        string
	NativeCompat   string
	Token          string
	TargetPath     string
	TempDir        string
}

type appState struct {
	AppName        string   `json:"appName"`
	SourceLabel    string   `json:"sourceLabel"`
	BaseURL        string   `json:"baseURL,omitempty"`
	ManifestURL    string   `json:"manifestURL"`
	CurrentVersion string   `json:"currentVersion"`
	CurrentHash    string   `json:"currentHash,omitempty"`
	Channel        string   `json:"channel"`
	NativeCompat   string   `json:"nativeCompat,omitempty"`
	AuthConfigured bool     `json:"authConfigured"`
	TargetPath     string   `json:"targetPath"`
	TempDir        string   `json:"tempDir"`
	Notes          []string `json:"notes,omitempty"`
}

type updateView struct {
	Version      string `json:"version"`
	Channel      string `json:"channel"`
	ArtifactURL  string `json:"artifactURL"`
	ArtifactHash string `json:"artifactHash"`
	ArtifactSize int64  `json:"artifactSize"`
	DeltaURL     string `json:"deltaURL,omitempty"`
	DeltaHash    string `json:"deltaHash,omitempty"`
	DeltaSize    int64  `json:"deltaSize,omitempty"`
	Mandatory    bool   `json:"mandatory"`
}

type checkResponse struct {
	CheckedAt string      `json:"checkedAt"`
	Available bool        `json:"available"`
	Update    *updateView `json:"update,omitempty"`
	Error     string      `json:"error,omitempty"`
}

type actionResponse struct {
	StartedAt string `json:"startedAt"`
	Applied   bool   `json:"applied"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

type logEvent struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	At      string `json:"at"`
}

type progressEvent struct {
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"`
}

type UpdateService struct {
	cfg     appConfig
	manager *update.Manager

	mu        sync.Mutex
	lastCheck *update.UpdateInfo
}

type authTransport struct {
	base  http.RoundTripper
	token string
}

func loadConfig() appConfig {
	targetPath, err := os.Executable()
	if err != nil {
		targetPath = ""
	}

	baseURL := strings.TrimRight(firstNonEmpty(os.Getenv("EXAMPLE_PROXY_BASE_URL"), "http://127.0.0.1:8787"), "/")
	manifestURL := strings.TrimSpace(os.Getenv("EXAMPLE_PROXY_MANIFEST_URL"))
	if manifestURL == "" && baseURL != "" {
		manifestURL = baseURL + "/manifest.json"
	}

	return appConfig{
		AppName:        "Authenticated HTTP Example",
		SourceLabel:    "HTTP proxy with bearer auth",
		BaseURL:        baseURL,
		ManifestURL:    manifestURL,
		CurrentVersion: firstNonEmpty(os.Getenv("EXAMPLE_CURRENT_VERSION"), appVersion),
		Channel:        firstNonEmpty(os.Getenv("EXAMPLE_UPDATE_CHANNEL"), "stable"),
		NativeCompat:   strings.TrimSpace(os.Getenv("EXAMPLE_NATIVE_COMPAT")),
		Token:          strings.TrimSpace(os.Getenv("EXAMPLE_PROXY_TOKEN")),
		TargetPath:     firstNonEmpty(os.Getenv("EXAMPLE_TARGET_PATH"), targetPath),
		TempDir:        firstNonEmpty(os.Getenv("EXAMPLE_TEMP_DIR"), filepath.Join(os.TempDir(), "wailsrel-http-example")),
	}
}

func NewUpdateService(cfg appConfig) *UpdateService {
	client := &http.Client{
		Timeout: 45 * time.Second,
		Transport: authTransport{
			base:  http.DefaultTransport,
			token: cfg.Token,
		},
	}
	checker := update.NewChecker(client)
	applier := update.NewApplier(update.ApplierOptions{
		Client:     client,
		TargetPath: cfg.TargetPath,
		TempDir:    cfg.TempDir,
	})

	service := &UpdateService{cfg: cfg}
	service.manager = update.NewManager(update.ManagerOpts{
		Checker: checker,
		Applier: applier,
		OnAvailable: func(result update.CheckResult) {
			if result.Native != nil {
				service.emit("info", fmt.Sprintf("Proxy exposed update %s.", result.Native.Version))
				service.emitAvailable(service.toView(result.Native))
			}
		},
		OnProgress: func(downloaded, total int64) {
			app := application.Get()
			if app == nil {
				return
			}
			app.Event.Emit("update:progress", progressEvent{
				Downloaded: downloaded,
				Total:      total,
			})
		},
		OnError: func(err error) {
			service.emit("error", err.Error())
		},
		OnRestart: func() error {
			service.emit("info", "Update installed through the authenticated proxy. Restart the app to launch the new binary.")
			return nil
		},
	})
	return service
}

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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

func (s *UpdateService) GetState() appState {
	currentHash, hashErr := s.currentHash()
	notes := []string{
		"EXAMPLE_PROXY_TOKEN is sent as a bearer token on all updater HTTP requests from this app.",
		"The Bun proxy keeps manifests stable at /manifest.json and rewrites artifact URLs back to /download/{tag}/{asset_name}.",
	}
	if strings.TrimSpace(s.cfg.Token) == "" {
		notes = append(notes, "EXAMPLE_PROXY_TOKEN is currently empty. The proxy can still serve manifests, but protected downloads will fail with 401.")
	}
	if hashErr != nil {
		notes = append(notes, "Current executable checksum is unavailable: "+hashErr.Error())
	}

	return appState{
		AppName:        s.cfg.AppName,
		SourceLabel:    s.cfg.SourceLabel,
		BaseURL:        s.cfg.BaseURL,
		ManifestURL:    s.cfg.ManifestURL,
		CurrentVersion: s.cfg.CurrentVersion,
		CurrentHash:    currentHash,
		Channel:        s.cfg.Channel,
		NativeCompat:   s.cfg.NativeCompat,
		AuthConfigured: strings.TrimSpace(s.cfg.Token) != "",
		TargetPath:     s.cfg.TargetPath,
		TempDir:        s.cfg.TempDir,
		Notes:          notes,
	}
}

func (s *UpdateService) CheckNow() checkResponse {
	response := checkResponse{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := s.validate(); err != nil {
		response.Error = err.Error()
		s.emit("error", err.Error())
		return response
	}

	currentHash, hashErr := s.currentHash()
	if hashErr != nil {
		s.emit("warn", "Continuing without a current executable checksum: "+hashErr.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := s.manager.CheckNow(ctx)
	if err != nil {
		response.Error = err.Error()
		return response
	}

	if result == nil || !result.Available || result.Native == nil {
		s.mu.Lock()
		s.lastCheck = nil
		s.mu.Unlock()
		s.emit("info", "No newer release was found on the authenticated HTTP endpoint.")
		return response
	}

	s.mu.Lock()
	s.lastCheck = result.Native
	s.mu.Unlock()

	response.Available = true
	response.Update = s.toView(result.Native)
	if currentHash == "" {
		s.emit("info", "Update metadata loaded. Delta selection is skipped until the local executable hash matches a published artifact.")
	}
	return response
}

func (s *UpdateService) ApplyLastUpdate() actionResponse {
	response := actionResponse{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   "No update has been applied.",
	}

	if err := s.validate(); err != nil {
		response.Error = err.Error()
		response.Message = "Configuration is incomplete."
		s.emit("error", err.Error())
		return response
	}

	info := s.latestUpdate()
	if info == nil {
		check := s.CheckNow()
		if check.Error != "" {
			response.Error = check.Error
			response.Message = "Unable to load an update before applying."
			return response
		}
		info = s.latestUpdate()
	}
	if info == nil {
		response.Message = "There is no available update to apply."
		s.emit("info", response.Message)
		return response
	}

	s.emit("info", fmt.Sprintf("Downloading %s from proxy URL %s", info.Version, info.ArtifactURL))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := s.manager.Apply(ctx, info); err != nil {
		response.Error = err.Error()
		response.Message = "Update apply failed."
		return response
	}

	response.Applied = true
	response.Message = "Update downloaded through the proxy and copied into the target path. Restart the app to load it."
	return response
}

func (s *UpdateService) latestUpdate() *update.UpdateInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastCheck == nil {
		return nil
	}
	cloned := *s.lastCheck
	return &cloned
}

func (s *UpdateService) validate() error {
	if strings.TrimSpace(s.cfg.ManifestURL) == "" {
		return fmt.Errorf("manifest URL is required")
	}
	if strings.TrimSpace(s.cfg.CurrentVersion) == "" {
		return fmt.Errorf("current version is required")
	}
	if strings.TrimSpace(s.cfg.TargetPath) == "" {
		return fmt.Errorf("target path is required")
	}
	return nil
}

func (s *UpdateService) currentHash() (string, error) {
	if strings.TrimSpace(s.cfg.TargetPath) == "" {
		return "", fmt.Errorf("target path is empty")
	}
	checksum, err := build.ComputeChecksum(s.cfg.TargetPath)
	if err != nil {
		return "", err
	}
	return "sha256:" + checksum, nil
}

func (s *UpdateService) toView(info *update.UpdateInfo) *updateView {
	if info == nil {
		return nil
	}
	return &updateView{
		Version:      info.Version,
		Channel:      info.Channel,
		ArtifactURL:  info.ArtifactURL,
		ArtifactHash: info.ArtifactHash,
		ArtifactSize: info.ArtifactSize,
		DeltaURL:     info.DeltaURL,
		DeltaHash:    info.DeltaHash,
		DeltaSize:    info.DeltaSize,
		Mandatory:    info.Mandatory,
	}
}

func (s *UpdateService) emit(level, message string) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit("update:log", logEvent{
		Level:   level,
		Message: message,
		At:      time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *UpdateService) emitAvailable(view *updateView) {
	if view == nil {
		return
	}
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit("update:available", *view)
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
