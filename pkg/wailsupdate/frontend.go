package wailsupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/Eriyc/wailsrel/pkg/frontend"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type FrontendState struct {
	Enabled              bool                    `json:"enabled"`
	CatalogURL           string                  `json:"catalogURL,omitempty"`
	LastCheckedAt        string                  `json:"lastCheckedAt,omitempty"`
	LastError            string                  `json:"lastError,omitempty"`
	Offline              bool                    `json:"offline"`
	Stale                bool                    `json:"stale"`
	ActiveMode           string                  `json:"activeMode"`
	Selection            string                  `json:"selection,omitempty"`
	ActiveBundle         *FrontendBundleView     `json:"activeBundle,omitempty"`
	InstalledCodepush    *FrontendBundleView     `json:"installedCodepush,omitempty"`
	InstalledExperiments []FrontendBundleView    `json:"installedExperiments,omitempty"`
	AvailableCodepush    *FrontendCodepushView   `json:"availableCodepush,omitempty"`
	AvailableExperiments []FrontendExperimentView `json:"availableExperiments,omitempty"`
}

type FrontendBundleView struct {
	Kind         string `json:"kind,omitempty"`
	Name         string `json:"name,omitempty"`
	Version      string `json:"version,omitempty"`
	CompatID     string `json:"compatID,omitempty"`
	Channel      string `json:"channel,omitempty"`
	SourceBranch string `json:"sourceBranch,omitempty"`
	CommitSHA    string `json:"commitSHA,omitempty"`
}

type FrontendCodepushView struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	CompatID    string `json:"compatID,omitempty"`
	URL         string `json:"url"`
	Checksum    string `json:"checksum"`
	Size        int64  `json:"size"`
	Force       bool   `json:"force"`
	PublishedAt string `json:"publishedAt"`
}

type FrontendExperimentView struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	CompatID    string `json:"compatID,omitempty"`
	URL         string `json:"url"`
	Checksum    string `json:"checksum"`
	Size        int64  `json:"size"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	PublishedAt string `json:"publishedAt"`
}

func (s *Service) GetFrontendState() FrontendState {
	return s.snapshotFrontendState()
}

func (s *Service) RefreshFrontendCatalog() FrontendState {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.opts.CheckTimeout)
	defer cancel()
	_ = s.refreshFrontendCatalogLocked(ctx)
	return s.snapshotFrontendState()
}

func (s *Service) ApplyCodepush() ActionResponse {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	response := ActionResponse{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   "No codepush has been applied.",
	}
	if err := s.validateFrontend(); err != nil {
		response.Error = err.Error()
		response.Message = "Frontend configuration is incomplete."
		s.setFrontendError(time.Now().UTC().Format(time.RFC3339), err)
		return response
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.opts.ApplyTimeout)
	defer cancel()
	if err := s.ensureFrontendCatalogLocked(ctx); err != nil {
		response.Error = err.Error()
		response.Message = "Unable to load the frontend catalog."
		return response
	}

	entry := s.cloneAvailableCodepush()
	if entry == nil {
		response.Message = "There is no compatible codepush available."
		return response
	}
	if err := s.applyCodepushEntryLocked(ctx, *entry); err != nil {
		response.Error = err.Error()
		response.Message = "Codepush apply failed."
		s.setFrontendError(time.Now().UTC().Format(time.RFC3339), err)
		return response
	}

	response.Applied = true
	response.Message = "Codepush installed. Reloading the frontend."
	s.emitFrontendReloadRequired()
	return response
}

func (s *Service) SwitchExperiment(name string) ActionResponse {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	response := ActionResponse{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   "Experiment switch was not applied.",
	}
	if err := s.validateFrontend(); err != nil {
		response.Error = err.Error()
		response.Message = "Frontend configuration is incomplete."
		return response
	}
	if err := frontend.ValidateVariantName(name); err != nil {
		response.Error = err.Error()
		response.Message = "Experiment switch was rejected."
		return response
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.opts.ApplyTimeout)
	defer cancel()
	if err := s.ensureFrontendCatalogLocked(ctx); err != nil {
		response.Error = err.Error()
		response.Message = "Unable to load the frontend catalog."
		return response
	}

	entry, err := s.findExperimentEntryLocked(name)
	if err != nil {
		response.Error = err.Error()
		response.Message = "Experiment is unavailable."
		return response
	}
	if err := s.installExperimentEntryLocked(ctx, entry); err != nil {
		response.Error = err.Error()
		response.Message = "Experiment install failed."
		s.setFrontendError(time.Now().UTC().Format(time.RFC3339), err)
		return response
	}
	if err := s.opts.FrontendManager.SetExperiment(name); err != nil {
		response.Error = err.Error()
		response.Message = "Experiment selection failed."
		return response
	}

	response.Applied = true
	response.Message = "Experiment selected. Reloading the frontend."
	s.emitFrontendReloadRequired()
	return response
}

func (s *Service) ResetFrontend() ActionResponse {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	response := ActionResponse{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   "Frontend is already using codepush or embedded assets.",
	}
	if s.opts.FrontendManager == nil {
		response.Error = "frontend manager is required"
		response.Message = "Frontend reset failed."
		return response
	}

	selection, _ := s.opts.FrontendManager.GetSelection()
	if selection == "" {
		return response
	}
	if err := s.opts.FrontendManager.ClearExperiment(); err != nil {
		response.Error = err.Error()
		response.Message = "Frontend reset failed."
		return response
	}

	response.Applied = true
	response.Message = "Frontend reset. Reloading the frontend."
	s.emitFrontendReloadRequired()
	return response
}

func (s *Service) validateFrontend() error {
	if s.opts.FrontendManager == nil {
		return fmt.Errorf("frontend manager is required")
	}
	if strings.TrimSpace(s.opts.FrontendCatalogURL) == "" {
		return fmt.Errorf("frontend catalog URL is required")
	}
	if strings.TrimSpace(s.opts.FrontendCatalogPublicKey) == "" {
		return fmt.Errorf("frontend catalog public key is required")
	}
	if _, err := parsePinnedCatalogURL(s.opts.FrontendCatalogURL); err != nil {
		return err
	}
	return nil
}

func (s *Service) snapshotFrontendState() FrontendState {
	state := FrontendState{
		Enabled:    s.opts.FrontendManager != nil,
		CatalogURL: s.opts.FrontendCatalogURL,
		ActiveMode: "embedded",
	}
	if s.opts.FrontendManager == nil {
		return state
	}

	selection, _ := s.opts.FrontendManager.GetSelection()
	state.Selection = selection

	if _, manifest, err := s.opts.FrontendManager.LoadEffective(); err == nil && manifest != nil {
		state.ActiveBundle = toFrontendBundleView(manifest)
		switch strings.TrimSpace(manifest.Kind) {
		case frontend.BundleKindExperiment:
			state.ActiveMode = frontend.BundleKindExperiment
		case frontend.BundleKindCodepush:
			state.ActiveMode = frontend.BundleKindCodepush
		default:
			state.ActiveMode = "legacy"
		}
	}
	if manifest, _ := s.opts.FrontendManager.LoadInstalledCodepush(); manifest != nil {
		state.InstalledCodepush = toFrontendBundleView(manifest)
	}
	if installed, _ := s.opts.FrontendManager.ListInstalledExperiments(); len(installed) > 0 {
		state.InstalledExperiments = make([]FrontendBundleView, 0, len(installed))
		for _, manifest := range installed {
			state.InstalledExperiments = append(state.InstalledExperiments, *toFrontendBundleView(&manifest))
		}
	}

	s.mu.Lock()
	state.LastCheckedAt = s.frontendLastCheckedAt
	state.LastError = s.frontendLastError
	state.Offline = s.frontendOffline
	state.Stale = s.frontendStale
	if s.frontendAvailableCodepush != nil {
		state.AvailableCodepush = toFrontendCodepushView(s.frontendAvailableCodepush)
	}
	if len(s.frontendExperiments) > 0 {
		state.AvailableExperiments = make([]FrontendExperimentView, 0, len(s.frontendExperiments))
		for _, entry := range s.frontendExperiments {
			state.AvailableExperiments = append(state.AvailableExperiments, *toFrontendExperimentView(&entry))
		}
	}
	s.mu.Unlock()
	return state
}

func (s *Service) ensureFrontendCatalogLocked(ctx context.Context) error {
	s.mu.Lock()
	hasCatalog := s.frontendCatalog != nil
	s.mu.Unlock()
	if hasCatalog {
		return nil
	}
	return s.refreshFrontendCatalogLocked(ctx)
}

func (s *Service) refreshFrontendCatalogLocked(ctx context.Context) error {
	if err := s.validateFrontend(); err != nil {
		s.setFrontendError(time.Now().UTC().Format(time.RFC3339), err)
		return err
	}

	catalog, checkedAt, err := s.fetchFrontendCatalog(ctx)
	if err != nil {
		s.setFrontendError(checkedAt, err)
		return err
	}

	experiments := frontend.CompatibleExperiments(catalog.Experiments, s.opts.NativeCompat)
	codepush := frontend.SelectNewestCodepush(catalog.Codepush, s.opts.NativeCompat)

	selection, _ := s.opts.FrontendManager.GetSelection()
	if selection != "" && !containsExperiment(experiments, selection) {
		if err := s.opts.FrontendManager.ClearExperiment(); err == nil {
			s.emitFrontendReloadRequired()
		}
	}

	s.setFrontendCatalogSnapshot(checkedAt, catalog, codepush, experiments)

	if codepush != nil && codepush.Force {
		if err := s.autoApplyForcedCodepushLocked(ctx, *codepush); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) fetchFrontendCatalog(ctx context.Context) (*frontend.Catalog, string, error) {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	pinnedURL, err := parsePinnedCatalogURL(s.opts.FrontendCatalogURL)
	if err != nil {
		return nil, checkedAt, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pinnedURL.String(), nil)
	if err != nil {
		return nil, checkedAt, err
	}
	resp, err := s.opts.Client.Do(req)
	if err != nil {
		return nil, checkedAt, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, checkedAt, fmt.Errorf("fetch frontend catalog: unexpected status %s", resp.Status)
	}
	if resp.Request == nil || resp.Request.URL == nil {
		return nil, checkedAt, fmt.Errorf("fetch frontend catalog: missing response url")
	}
	if !sameOrigin(pinnedURL, resp.Request.URL) {
		return nil, checkedAt, fmt.Errorf("fetch frontend catalog: redirected away from pinned origin")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, checkedAt, err
	}
	catalog, err := frontend.DecodeCatalog(data, s.opts.FrontendCatalogPublicKey, s.opts.FrontendManager.AppID)
	if err != nil {
		return nil, checkedAt, err
	}
	return catalog, checkedAt, nil
}

func (s *Service) autoApplyForcedCodepushLocked(ctx context.Context, entry frontend.CodepushEntry) error {
	failed, err := s.opts.FrontendManager.HasForcedCodepushFailure(entry.Version)
	if err != nil || failed {
		return err
	}
	if installed, _ := s.opts.FrontendManager.LoadInstalledCodepush(); installed != nil &&
		installed.Name == entry.Name &&
		installed.Version == entry.Version &&
		installed.Checksum == entry.Checksum {
		return nil
	}

	if err := s.applyCodepushEntryLocked(ctx, entry); err != nil {
		_ = s.opts.FrontendManager.RecordForcedCodepushFailure(entry.Version)
		s.setFrontendError(time.Now().UTC().Format(time.RFC3339), err)
		return err
	}
	_ = s.opts.FrontendManager.ClearForcedCodepushFailure(entry.Version)
	s.emitFrontendReloadRequired()
	return nil
}

func (s *Service) applyCodepushEntryLocked(ctx context.Context, entry frontend.CodepushEntry) error {
	return s.downloadAndInstallFrontendBundle(ctx, entry.URL, entry.Checksum, func(file *os.File) error {
		if _, err := file.Seek(0, 0); err != nil {
			return err
		}
		return s.opts.FrontendManager.InstallCodepush(ctx, file)
	})
}

func (s *Service) installExperimentEntryLocked(ctx context.Context, entry frontend.ExperimentEntry) error {
	if installed, _ := s.opts.FrontendManager.LoadInstalledExperiment(entry.Name); installed != nil &&
		installed.Version == entry.Version &&
		installed.Checksum == entry.Checksum {
		return nil
	}
	return s.downloadAndInstallFrontendBundle(ctx, entry.URL, entry.Checksum, func(file *os.File) error {
		if _, err := file.Seek(0, 0); err != nil {
			return err
		}
		return s.opts.FrontendManager.InstallExperiment(ctx, entry.Name, file)
	})
}

func (s *Service) downloadAndInstallFrontendBundle(ctx context.Context, sourceURL, expectedChecksum string, install func(*os.File) error) error {
	if err := os.MkdirAll(s.opts.TempDir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.opts.TempDir, "frontend-catalog-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.opts.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download frontend bundle: unexpected status %s", resp.Status)
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	checksum, err := build.ComputeChecksum(file.Name())
	if err != nil {
		return err
	}
	if "sha256:"+checksum != expectedChecksum {
		return fmt.Errorf("frontend bundle checksum mismatch for %s", sourceURL)
	}
	opened, err := os.Open(file.Name())
	if err != nil {
		return err
	}
	defer opened.Close()
	return install(opened)
}

func (s *Service) findExperimentEntryLocked(name string) (frontend.ExperimentEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.frontendExperiments {
		if entry.Name == name {
			return entry, nil
		}
	}
	return frontend.ExperimentEntry{}, fmt.Errorf("experiment %q is not available", name)
}

func (s *Service) cloneAvailableCodepush() *frontend.CodepushEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frontendAvailableCodepush == nil {
		return nil
	}
	copy := *s.frontendAvailableCodepush
	return &copy
}

func (s *Service) setFrontendCatalogSnapshot(checkedAt string, catalog *frontend.Catalog, codepush *frontend.CodepushEntry, experiments []frontend.ExperimentEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frontendCatalog = catalog
	if codepush != nil {
		copy := *codepush
		s.frontendAvailableCodepush = &copy
	} else {
		s.frontendAvailableCodepush = nil
	}
	s.frontendExperiments = append([]frontend.ExperimentEntry(nil), experiments...)
	s.frontendLastCheckedAt = checkedAt
	s.frontendLastError = ""
	s.frontendOffline = false
	s.frontendStale = false
}

func (s *Service) setFrontendError(checkedAt string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frontendLastCheckedAt = checkedAt
	if err != nil {
		s.frontendLastError = err.Error()
	}
	s.frontendOffline = true
	s.frontendStale = true
}

func (s *Service) emitFrontendReloadRequired() {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit(s.opts.EventPrefix+":frontend-reload-required", struct{}{})
}

func parsePinnedCatalogURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("frontend catalog URL is invalid: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("frontend catalog URL must use https")
	}
	return parsed, nil
}

func sameOrigin(left, right *url.URL) bool {
	return left != nil && right != nil && strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

func containsExperiment(entries []frontend.ExperimentEntry, name string) bool {
	for _, entry := range entries {
		if entry.Name == name {
			return true
		}
	}
	return false
}

func toFrontendBundleView(manifest *frontend.BundleManifest) *FrontendBundleView {
	if manifest == nil {
		return nil
	}
	return &FrontendBundleView{
		Kind:         manifest.Kind,
		Name:         manifest.Name,
		Version:      manifest.Version,
		CompatID:     manifest.CompatID,
		Channel:      manifest.Channel,
		SourceBranch: manifest.SourceBranch,
		CommitSHA:    manifest.CommitSHA,
	}
}

func toFrontendCodepushView(entry *frontend.CodepushEntry) *FrontendCodepushView {
	if entry == nil {
		return nil
	}
	return &FrontendCodepushView{
		Name:        entry.Name,
		Version:     entry.Version,
		CompatID:    entry.CompatID,
		URL:         entry.URL,
		Checksum:    entry.Checksum,
		Size:        entry.Size,
		Force:       entry.Force,
		PublishedAt: entry.PublishedAt.UTC().Format(time.RFC3339),
	}
}

func toFrontendExperimentView(entry *frontend.ExperimentEntry) *FrontendExperimentView {
	if entry == nil {
		return nil
	}
	return &FrontendExperimentView{
		Name:        entry.Name,
		Version:     entry.Version,
		CompatID:    entry.CompatID,
		URL:         entry.URL,
		Checksum:    entry.Checksum,
		Size:        entry.Size,
		DisplayName: entry.DisplayName,
		Description: entry.Description,
		PublishedAt: entry.PublishedAt.UTC().Format(time.RFC3339),
	}
}
