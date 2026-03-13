package frontend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type BundleManager struct {
	AppID        string
	NativeCompat string
	OverrideRoot string

	mu sync.RWMutex
}

type bundleSelection struct {
	Experiment string    `json:"experiment"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type installRecord struct {
	Kind         string    `json:"kind"`
	Name         string    `json:"name"`
	Version      string    `json:"version,omitempty"`
	CompatID     string    `json:"compat_id,omitempty"`
	Checksum     string    `json:"checksum"`
	SourceBranch string    `json:"source_branch,omitempty"`
	CommitSHA    string    `json:"commit_sha,omitempty"`
	InstalledAt  time.Time `json:"installed_at"`
}

type forcedFailures struct {
	Versions map[string]string `json:"versions,omitempty"`
}

func (bm *BundleManager) ActiveChannel() string {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.activeChannelLocked()
}

func (bm *BundleManager) SetChannel(channel string) error {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return fmt.Errorf("channel is required")
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	if err := os.MkdirAll(bm.rootDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(bm.channelMarkerPath(), []byte(channel+"\n"), 0o644)
}

func (bm *BundleManager) Install(ctx context.Context, channel string, bundle io.Reader) error {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return fmt.Errorf("channel is required")
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	manifest, err := bm.installBundleLocked(ctx, bundle, installBundleOptions{
		FinalDir:         bm.channelDir(channel),
		ExpectedChannel:  channel,
		RequireTrustFile: false,
	})
	if err != nil {
		return err
	}
	if manifest.Channel != "" && manifest.Channel != channel {
		return fmt.Errorf("bundle channel %q does not match requested channel %q", manifest.Channel, channel)
	}
	return nil
}

func (bm *BundleManager) InstallCodepush(ctx context.Context, bundle io.Reader) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	manifest, err := bm.installBundleLocked(ctx, bundle, installBundleOptions{
		FinalDir:         bm.codepushCurrentDir(),
		ExpectedKind:     BundleKindCodepush,
		RequireTrustFile: true,
	})
	if err != nil {
		return err
	}
	return bm.pruneStaleInstallRecordsLocked(BundleKindCodepush, manifest.Name)
}

func (bm *BundleManager) InstallExperiment(ctx context.Context, name string, bundle io.Reader) error {
	if err := ValidateVariantName(name); err != nil {
		return err
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	_, err := bm.installBundleLocked(ctx, bundle, installBundleOptions{
		FinalDir:         bm.experimentDir(name),
		ExpectedKind:     BundleKindExperiment,
		ExpectedName:     name,
		RequireTrustFile: true,
	})
	return err
}

func (bm *BundleManager) GetSelection() (string, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.selectionLocked()
}

func (bm *BundleManager) SetExperiment(name string) error {
	if err := ValidateVariantName(name); err != nil {
		return err
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	if _, err := bm.validateInstalledBundleLocked(bm.experimentDir(name), BundleKindExperiment, name, true); err != nil {
		return err
	}
	return bm.writeSelectionLocked(name)
}

func (bm *BundleManager) ClearExperiment() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.clearSelectionLocked()
}

func (bm *BundleManager) ClearInvalidSelection() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	name, err := bm.selectionLocked()
	if err != nil || name == "" {
		return nil
	}
	if _, err := bm.validateInstalledBundleLocked(bm.experimentDir(name), BundleKindExperiment, name, true); err == nil {
		return nil
	}
	return bm.clearSelectionLocked()
}

func (bm *BundleManager) LoadInstalledCodepush() (*BundleManifest, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	manifest, err := bm.validateInstalledBundleLocked(bm.codepushCurrentDir(), BundleKindCodepush, "", true)
	if err != nil {
		return nil, nil
	}
	return manifest, nil
}

func (bm *BundleManager) LoadInstalledExperiment(name string) (*BundleManifest, error) {
	if err := ValidateVariantName(name); err != nil {
		return nil, err
	}

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	manifest, err := bm.validateInstalledBundleLocked(bm.experimentDir(name), BundleKindExperiment, name, true)
	if err != nil {
		return nil, nil
	}
	return manifest, nil
}

func (bm *BundleManager) LoadActive() (fs.FS, *BundleManifest, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	channel := bm.activeChannelLocked()
	manifest, err := bm.validateInstalledBundleLocked(bm.channelDir(channel), "", "", false)
	if err != nil {
		return nil, nil, nil
	}
	return os.DirFS(bm.channelDir(channel)), manifest, nil
}

func (bm *BundleManager) LoadEffective() (fs.FS, *BundleManifest, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	dir, manifest, clearSelection := bm.resolveEffectiveLocked()
	if clearSelection {
		_ = bm.clearSelectionLocked()
	}
	if manifest == nil || dir == "" {
		return nil, nil, nil
	}
	return os.DirFS(dir), manifest, nil
}

func (bm *BundleManager) ListInstalledExperiments() ([]BundleManifest, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	entries, err := os.ReadDir(bm.experimentsRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	manifests := make([]BundleManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := ValidateVariantName(entry.Name()); err != nil {
			continue
		}
		manifest, err := bm.validateInstalledBundleLocked(bm.experimentDir(entry.Name()), BundleKindExperiment, entry.Name(), true)
		if err != nil {
			continue
		}
		manifests = append(manifests, *manifest)
	}

	sort.Slice(manifests, func(i, j int) bool {
		if manifests[i].Name == manifests[j].Name {
			return manifests[i].Version < manifests[j].Version
		}
		return manifests[i].Name < manifests[j].Name
	})
	return manifests, nil
}

func (bm *BundleManager) ListInstalled() ([]BundleManifest, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	entries, err := os.ReadDir(bm.channelsRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	manifests := make([]BundleManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := bm.validateInstalledBundleLocked(filepath.Join(bm.channelsRoot(), entry.Name()), "", "", false)
		if err != nil {
			continue
		}
		manifests = append(manifests, *manifest)
	}

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].Channel < manifests[j].Channel
	})
	return manifests, nil
}

func (bm *BundleManager) Cleanup() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	entries, err := os.ReadDir(bm.channelsRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := bm.validateInstalledBundleLocked(filepath.Join(bm.channelsRoot(), entry.Name()), "", "", false); err != nil {
			if removeErr := os.RemoveAll(filepath.Join(bm.channelsRoot(), entry.Name())); removeErr != nil {
				return removeErr
			}
		}
	}
	return nil
}

func (bm *BundleManager) HasForcedCodepushFailure(version string) (bool, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return false, nil
	}

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	state, err := bm.loadForcedFailuresLocked()
	if err != nil {
		return false, err
	}
	_, ok := state.Versions[version]
	return ok, nil
}

func (bm *BundleManager) RecordForcedCodepushFailure(version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return fmt.Errorf("codepush version is required")
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	state, err := bm.loadForcedFailuresLocked()
	if err != nil {
		return err
	}
	if state.Versions == nil {
		state.Versions = map[string]string{}
	}
	state.Versions[version] = time.Now().UTC().Format(time.RFC3339)
	return bm.writeForcedFailuresLocked(state)
}

func (bm *BundleManager) ClearForcedCodepushFailure(version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	state, err := bm.loadForcedFailuresLocked()
	if err != nil {
		return err
	}
	delete(state.Versions, version)
	return bm.writeForcedFailuresLocked(state)
}

func (bm *BundleManager) rootDir() string {
	if strings.TrimSpace(bm.OverrideRoot) != "" {
		return bm.OverrideRoot
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", ".wailsrel", safePathPart(bm.AppID))
	}
	return filepath.Join(base, "wailsrel", safePathPart(bm.AppID), "frontend")
}

func (bm *BundleManager) channelsRoot() string {
	return filepath.Join(bm.rootDir(), "channels")
}

func (bm *BundleManager) channelDir(channel string) string {
	return filepath.Join(bm.channelsRoot(), safePathPart(channel))
}

func (bm *BundleManager) channelMarkerPath() string {
	return filepath.Join(bm.rootDir(), "frontend-channel")
}

func (bm *BundleManager) selectionPath() string {
	return filepath.Join(bm.rootDir(), "selection.json")
}

func (bm *BundleManager) codepushCurrentDir() string {
	return filepath.Join(bm.rootDir(), "codepush", "current")
}

func (bm *BundleManager) experimentsRoot() string {
	return filepath.Join(bm.rootDir(), "experiments")
}

func (bm *BundleManager) experimentDir(name string) string {
	return filepath.Join(bm.experimentsRoot(), name)
}

func (bm *BundleManager) installRecordsRoot() string {
	return filepath.Join(bm.rootDir(), "install-records")
}

func (bm *BundleManager) installRecordPath(kind, name string) string {
	return filepath.Join(bm.installRecordsRoot(), fmt.Sprintf("%s-%s.json", kind, name))
}

func (bm *BundleManager) forcedFailuresPath() string {
	return filepath.Join(bm.rootDir(), "forced-failures.json")
}

func (bm *BundleManager) activeChannelLocked() string {
	data, err := os.ReadFile(bm.channelMarkerPath())
	if err != nil {
		return "stable"
	}
	channel := strings.TrimSpace(string(data))
	if channel == "" {
		return "stable"
	}
	return channel
}

func (bm *BundleManager) selectionLocked() (string, error) {
	selection, err := bm.loadSelectionLocked()
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if err := ValidateVariantName(selection.Experiment); err != nil {
		return "", err
	}
	return selection.Experiment, nil
}

func (bm *BundleManager) writeSelectionLocked(name string) error {
	if err := os.MkdirAll(bm.rootDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bundleSelection{
		Experiment: name,
		UpdatedAt:  time.Now().UTC(),
	}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(bm.selectionPath(), data, 0o644)
}

func (bm *BundleManager) clearSelectionLocked() error {
	if err := os.Remove(bm.selectionPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (bm *BundleManager) loadSelectionLocked() (*bundleSelection, error) {
	data, err := os.ReadFile(bm.selectionPath())
	if err != nil {
		return nil, err
	}
	var selection bundleSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return nil, err
	}
	return &selection, nil
}

func (bm *BundleManager) resolveEffectiveLocked() (string, *BundleManifest, bool) {
	clearSelection := false
	name, err := bm.selectionLocked()
	if err == nil && name != "" {
		manifest, err := bm.validateInstalledBundleLocked(bm.experimentDir(name), BundleKindExperiment, name, true)
		if err == nil {
			return bm.experimentDir(name), manifest, false
		}
		clearSelection = true
	}
	if manifest, err := bm.validateInstalledBundleLocked(bm.codepushCurrentDir(), BundleKindCodepush, "", true); err == nil {
		return bm.codepushCurrentDir(), manifest, clearSelection
	}
	channel := bm.activeChannelLocked()
	if manifest, err := bm.validateInstalledBundleLocked(bm.channelDir(channel), "", "", false); err == nil {
		return bm.channelDir(channel), manifest, clearSelection
	}
	return "", nil, clearSelection
}

type installBundleOptions struct {
	FinalDir         string
	ExpectedKind     string
	ExpectedName     string
	ExpectedChannel  string
	RequireTrustFile bool
}

func (bm *BundleManager) installBundleLocked(ctx context.Context, bundle io.Reader, opts installBundleOptions) (*BundleManifest, error) {
	if err := os.MkdirAll(bm.rootDir(), 0o755); err != nil {
		return nil, err
	}

	tempFile, err := os.CreateTemp(bm.rootDir(), "bundle-*.zip")
	if err != nil {
		return nil, err
	}
	tempZipPath := tempFile.Name()
	defer os.Remove(tempZipPath)

	if _, err := io.Copy(tempFile, bundle); err != nil {
		tempFile.Close()
		return nil, err
	}
	if err := tempFile.Close(); err != nil {
		return nil, err
	}

	manifest, err := readBundleManifestFromZip(tempZipPath)
	if err != nil {
		return nil, err
	}
	if opts.ExpectedChannel != "" && manifest.Channel != "" && manifest.Channel != opts.ExpectedChannel {
		return nil, fmt.Errorf("bundle channel %q does not match requested channel %q", manifest.Channel, opts.ExpectedChannel)
	}
	if opts.ExpectedKind != "" {
		if strings.TrimSpace(manifest.Kind) != opts.ExpectedKind {
			return nil, fmt.Errorf("bundle kind %q does not match requested kind %q", manifest.Kind, opts.ExpectedKind)
		}
		if err := ValidateVariantName(manifest.Name); err != nil {
			return nil, err
		}
		if opts.ExpectedName != "" && manifest.Name != opts.ExpectedName {
			return nil, fmt.Errorf("bundle name %q does not match requested name %q", manifest.Name, opts.ExpectedName)
		}
	}
	if err := CheckCompat(*manifest, bm.NativeCompat); err != nil {
		return nil, err
	}
	if manifest.Checksum != "" {
		if err := ValidateBundleChecksum(manifest.Checksum); err != nil {
			return nil, err
		}
	}

	stagingDir, err := os.MkdirTemp(bm.rootDir(), "bundle-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stagingDir)

	if err := extractBundleArchive(tempZipPath, stagingDir); err != nil {
		return nil, err
	}
	if err := writeBundleManifest(filepath.Join(stagingDir, "bundle.json"), *manifest); err != nil {
		return nil, err
	}
	if manifest.Checksum != "" {
		checksum, err := computeBundleDirectoryChecksum(stagingDir)
		if err != nil {
			return nil, err
		}
		if withSHA256Prefix(checksum) != withSHA256Prefix(manifest.Checksum) {
			return nil, fmt.Errorf("bundle checksum mismatch for %s", opts.FinalDir)
		}
	}

	finalDir := opts.FinalDir
	backupDir := finalDir + ".old"
	_ = os.RemoveAll(backupDir)
	if _, err := os.Stat(finalDir); err == nil {
		if err := os.Rename(finalDir, backupDir); err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		_ = os.Rename(backupDir, finalDir)
		return nil, err
	}
	if err := os.Rename(stagingDir, finalDir); err != nil {
		_ = os.Rename(backupDir, finalDir)
		return nil, err
	}
	_ = os.RemoveAll(backupDir)

	if opts.RequireTrustFile {
		if err := bm.writeInstallRecordLocked(*manifest); err != nil {
			return nil, err
		}
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return manifest, nil
	}
}

func (bm *BundleManager) validateInstalledBundleLocked(dir, expectedKind, expectedName string, requireTrustFile bool) (*BundleManifest, error) {
	manifest, err := loadBundleManifest(filepath.Join(dir, "bundle.json"))
	if err != nil {
		return nil, err
	}
	if expectedKind != "" && strings.TrimSpace(manifest.Kind) != expectedKind {
		return nil, fmt.Errorf("bundle kind %q does not match expected kind %q", manifest.Kind, expectedKind)
	}
	if expectedName != "" && strings.TrimSpace(manifest.Name) != expectedName {
		return nil, fmt.Errorf("bundle name %q does not match expected name %q", manifest.Name, expectedName)
	}
	if expectedKind != "" {
		if err := ValidateVariantName(manifest.Name); err != nil {
			return nil, err
		}
		if err := ValidateBundleChecksum(manifest.Checksum); err != nil {
			return nil, err
		}
		record, err := bm.loadInstallRecordLocked(manifest.Kind, manifest.Name)
		if err != nil {
			return nil, err
		}
		if requireTrustFile {
			if record.Kind != manifest.Kind || record.Name != manifest.Name {
				return nil, fmt.Errorf("install record mismatch for %s/%s", manifest.Kind, manifest.Name)
			}
			if withSHA256Prefix(record.Checksum) != withSHA256Prefix(manifest.Checksum) {
				return nil, fmt.Errorf("install record checksum mismatch for %s/%s", manifest.Kind, manifest.Name)
			}
			if strings.TrimSpace(record.Version) != "" && strings.TrimSpace(record.Version) != strings.TrimSpace(manifest.Version) {
				return nil, fmt.Errorf("install record version mismatch for %s/%s", manifest.Kind, manifest.Name)
			}
		}
	}
	if err := CheckCompat(*manifest, bm.NativeCompat); err != nil {
		return nil, err
	}
	if manifest.Checksum != "" {
		checksum, err := computeBundleDirectoryChecksum(dir)
		if err != nil {
			return nil, err
		}
		if withSHA256Prefix(checksum) != withSHA256Prefix(manifest.Checksum) {
			return nil, fmt.Errorf("bundle checksum mismatch for %s", dir)
		}
	}
	return manifest, nil
}

func (bm *BundleManager) writeInstallRecordLocked(manifest BundleManifest) error {
	record := installRecord{
		Kind:         manifest.Kind,
		Name:         manifest.Name,
		Version:      manifest.Version,
		CompatID:     manifest.CompatID,
		Checksum:     manifest.Checksum,
		SourceBranch: manifest.SourceBranch,
		CommitSHA:    manifest.CommitSHA,
		InstalledAt:  time.Now().UTC(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(bm.installRecordsRoot(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(bm.installRecordPath(record.Kind, record.Name), data, 0o644)
}

func (bm *BundleManager) loadInstallRecordLocked(kind, name string) (*installRecord, error) {
	data, err := os.ReadFile(bm.installRecordPath(kind, name))
	if err != nil {
		return nil, err
	}
	var record installRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (bm *BundleManager) pruneStaleInstallRecordsLocked(kind, keepName string) error {
	entries, err := os.ReadDir(bm.installRecordsRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	prefix := kind + "-"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || entry.Name() == prefix+keepName+".json" {
			continue
		}
		if err := os.Remove(filepath.Join(bm.installRecordsRoot(), entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (bm *BundleManager) loadForcedFailuresLocked() (forcedFailures, error) {
	data, err := os.ReadFile(bm.forcedFailuresPath())
	if err != nil {
		if os.IsNotExist(err) {
			return forcedFailures{Versions: map[string]string{}}, nil
		}
		return forcedFailures{}, err
	}
	var state forcedFailures
	if err := json.Unmarshal(data, &state); err != nil {
		return forcedFailures{}, err
	}
	if state.Versions == nil {
		state.Versions = map[string]string{}
	}
	return state, nil
}

func (bm *BundleManager) writeForcedFailuresLocked(state forcedFailures) error {
	if len(state.Versions) == 0 {
		if err := os.Remove(bm.forcedFailuresPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(bm.rootDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(bm.forcedFailuresPath(), data, 0o644)
}

func computeBundleDirectoryChecksum(root string) (string, error) {
	hash := sha256.New()
	if err := hashBundleDirectory(hash, root); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashBundleDirectory(w hash.Hash, root string) error {
	paths := make([]string, 0)
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == "bundle.json" {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(w, filepath.ToSlash(rel)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, info.Mode().String()); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(w, target); err != nil {
				return err
			}
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
			continue
		}
		if info.IsDir() {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, file); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func withSHA256Prefix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "sha256:") {
		return "sha256:" + strings.TrimPrefix(strings.TrimPrefix(value, "sha256:"), "SHA256:")
	}
	return "sha256:" + value
}

func safePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}

	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return replacer.Replace(value)
}
