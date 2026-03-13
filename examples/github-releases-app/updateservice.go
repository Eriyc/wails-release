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
	Repository     string
	ManifestURL    string
	FrontendCatalogURL    string
	FrontendCatalogPublicKey string
	CurrentVersion string
	Channel        string
	NativeCompat   string
	TargetPath     string
	TempDir        string
}

type UpdateService struct {
	service         *wailsupdate.Service
	frontendManager *frontend.BundleManager
}

func loadConfig() appConfig {
	repository := strings.TrimSpace(os.Getenv("EXAMPLE_GITHUB_REPOSITORY"))
	manifestURL := strings.TrimSpace(os.Getenv("EXAMPLE_GITHUB_MANIFEST_URL"))
	if manifestURL == "" {
		manifestURL = wailsupdate.GitHubLatestManifestURL(repository)
	}

	return appConfig{
		AppName:        "GitHub Releases Example",
		SourceLabel:    "GitHub Releases (direct)",
		Repository:     repository,
		ManifestURL:    manifestURL,
		FrontendCatalogURL:    strings.TrimSpace(os.Getenv("EXAMPLE_FRONTEND_CATALOG_URL")),
		FrontendCatalogPublicKey: strings.TrimSpace(os.Getenv("EXAMPLE_FRONTEND_CATALOG_PUBLIC_KEY")),
		CurrentVersion: firstNonEmpty(os.Getenv("EXAMPLE_CURRENT_VERSION"), appVersion),
		Channel:        firstNonEmpty(os.Getenv("EXAMPLE_UPDATE_CHANNEL"), "stable"),
		NativeCompat:   strings.TrimSpace(os.Getenv("EXAMPLE_NATIVE_COMPAT")),
		TargetPath:     firstNonEmpty(os.Getenv("EXAMPLE_TARGET_PATH"), wailsupdate.DefaultTargetPath()),
		TempDir:        firstNonEmpty(os.Getenv("EXAMPLE_TEMP_DIR"), filepath.Join(os.TempDir(), "wailsrel-github-example")),
	}
}

func NewUpdateService(cfg appConfig) *UpdateService {
	client := &http.Client{Timeout: 45 * time.Second}
	frontendManager := &frontend.BundleManager{
		AppID:        "com.example.wailsrel.githubreleasesapp",
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
			if cfg.Repository != "" {
				state.Metadata["repository"] = cfg.Repository
			}
			state.Notes = append(state.Notes,
				"Set EXAMPLE_GITHUB_REPOSITORY or EXAMPLE_GITHUB_MANIFEST_URL before checking for updates.",
				"ApplyPending installs the matched frontend bundle first and stages the native binary when a native update is also available.",
				"Running with `go run` points the updater at a temporary Go build cache executable. Use packaged builds to validate apply and delta flows.",
			)
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
