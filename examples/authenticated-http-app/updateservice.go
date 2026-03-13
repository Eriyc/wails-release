package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Eriyc/wailsrel/pkg/frontend"
	"github.com/Eriyc/wailsrel/pkg/wailsupdate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var appVersion = "0.1.0"

type appConfig struct {
	AppName        string
	SourceLabel    string
	BaseURL        string
	ManifestURL    string
	FrontendCatalogURL       string
	FrontendCatalogPublicKey string
	CurrentVersion string
	Channel        string
	NativeCompat   string
	Token          string
	TargetPath     string
	TempDir        string
}

type UpdateService struct {
	service         *wailsupdate.Service
	frontendManager *frontend.BundleManager
}

func loadConfig() appConfig {
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
		FrontendCatalogURL:       strings.TrimSpace(os.Getenv("EXAMPLE_FRONTEND_CATALOG_URL")),
		FrontendCatalogPublicKey: strings.TrimSpace(os.Getenv("EXAMPLE_FRONTEND_CATALOG_PUBLIC_KEY")),
		CurrentVersion: firstNonEmpty(os.Getenv("EXAMPLE_CURRENT_VERSION"), appVersion),
		Channel:        firstNonEmpty(os.Getenv("EXAMPLE_UPDATE_CHANNEL"), "stable"),
		NativeCompat:   strings.TrimSpace(os.Getenv("EXAMPLE_NATIVE_COMPAT")),
		Token:          strings.TrimSpace(os.Getenv("EXAMPLE_PROXY_TOKEN")),
		TargetPath:     firstNonEmpty(os.Getenv("EXAMPLE_TARGET_PATH"), wailsupdate.DefaultTargetPath()),
		TempDir:        firstNonEmpty(os.Getenv("EXAMPLE_TEMP_DIR"), filepath.Join(os.TempDir(), "wailsrel-http-example")),
	}
}

func NewUpdateService(cfg appConfig) *UpdateService {
	client := &http.Client{
		Timeout:   45 * time.Second,
		Transport: wailsupdate.BearerTransport(nil, cfg.Token),
	}
	frontendManager := &frontend.BundleManager{
		AppID:        "com.example.wailsrel.authenticatedhttpapp",
		NativeCompat: cfg.NativeCompat,
	}

	service := wailsupdate.NewService(wailsupdate.Options{
		ManifestURL:    cfg.ManifestURL,
		FrontendCatalogURL: cfg.FrontendCatalogURL,
		FrontendCatalogPublicKey: cfg.FrontendCatalogPublicKey,
		CurrentVersion: cfg.CurrentVersion,
		Channel:        cfg.Channel,
		NativeCompat:   cfg.NativeCompat,
		TargetPath:     cfg.TargetPath,
		TempDir:        cfg.TempDir,
		Client:         client,
		FrontendManager: frontendManager,
		DescribeState: func(state *wailsupdate.State) {
			if state.Metadata == nil {
				state.Metadata = map[string]string{}
			}
			state.Metadata["appName"] = cfg.AppName
			state.Metadata["sourceLabel"] = cfg.SourceLabel
			state.Metadata["baseURL"] = cfg.BaseURL
			state.Metadata["authConfigured"] = boolString(cfg.Token != "")
			state.Notes = append(state.Notes,
				"EXAMPLE_PROXY_TOKEN is sent as a bearer token on all updater HTTP requests from this app.",
				"The Bun proxy keeps manifests stable at /manifest.json and rewrites artifact URLs back to /download/{tag}/{asset_name}.",
				"ApplyPending installs the matched frontend bundle first and stages the native binary when a native update is also available.",
			)
			if strings.TrimSpace(cfg.Token) == "" {
				state.Notes = append(state.Notes, "EXAMPLE_PROXY_TOKEN is currently empty. The proxy can still serve manifests, but protected downloads will fail with 401.")
			}
		},
	})

	return &UpdateService{service: service, frontendManager: frontendManager}
}

func (s *UpdateService) GetState() wailsupdate.State {
	return s.service.GetState()
}

func (s *UpdateService) CheckNow() wailsupdate.CheckResponse {
	return s.service.CheckNow()
}

func (s *UpdateService) ApplyPending() wailsupdate.ActionResponse {
	return s.service.ApplyPending()
}

func (s *UpdateService) Restart() wailsupdate.ActionResponse {
	return s.service.Restart()
}

func (s *UpdateService) GetFrontendState() wailsupdate.FrontendState {
	return s.service.GetFrontendState()
}

func (s *UpdateService) RefreshFrontendCatalog() wailsupdate.FrontendState {
	return s.service.RefreshFrontendCatalog()
}

func (s *UpdateService) ApplyCodepush() wailsupdate.ActionResponse {
	return s.service.ApplyCodepush()
}

func (s *UpdateService) SwitchExperiment(name string) wailsupdate.ActionResponse {
	return s.service.SwitchExperiment(name)
}

func (s *UpdateService) ResetFrontend() wailsupdate.ActionResponse {
	return s.service.ResetFrontend()
}

func (s *UpdateService) FrontendManager() *frontend.BundleManager {
	return s.frontendManager
}

func (s *UpdateService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return s.service.ServiceStartup(ctx, options)
}

func (s *UpdateService) ServiceShutdown() error {
	return s.service.ServiceShutdown()
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

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
