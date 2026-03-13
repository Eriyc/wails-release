package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/Eriyc/wailsrel/pkg/contract"
	"github.com/Eriyc/wailsrel/pkg/delta"
	"github.com/Eriyc/wailsrel/pkg/release"
	"github.com/Eriyc/wailsrel/pkg/version"
)

type UpdateInfo struct {
	Version       string
	Channel       string
	ReleaseNotes  string
	Mandatory     bool
	ArtifactURL   string
	ArtifactHash  string
	ArtifactSize  int64
	DeltaURL      string
	DeltaHash     string
	DeltaSize     int64
	DeltaFromHash string
	Frontend      *FrontendUpdateInfo
}

type FrontendUpdateInfo struct {
	Channel  string
	Version  string
	CompatID string
	URL      string
	Hash     string
	Size     int64
}

type CheckResult struct {
	Available bool
	Native    *UpdateInfo
	Frontend  *FrontendUpdateInfo
}

type ProgressFunc func(downloaded, total int64)

type Checker interface {
	Check(ctx context.Context, opts CheckOpts) (*CheckResult, error)
}

type CheckOpts struct {
	CurrentVersion string
	CurrentHash    string
	NativeCompat   string
	Channel        string
	ManifestURL    string
}

type HTTPChecker struct {
	client *http.Client
}

func NewChecker(client *http.Client) *HTTPChecker {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPChecker{client: client}
}

func (c *HTTPChecker) Check(ctx context.Context, opts CheckOpts) (*CheckResult, error) {
	if strings.TrimSpace(opts.ManifestURL) == "" {
		return nil, fmt.Errorf("manifest url is required")
	}

	manifest, err := c.fetchReleaseManifest(ctx, opts.ManifestURL)
	if err != nil {
		return nil, err
	}

	currentVersion, err := version.Parse(opts.CurrentVersion)
	if err != nil {
		return nil, err
	}
	releaseVersion, err := version.Parse(manifest.Release.Version)
	if err != nil {
		return nil, err
	}

	result := &CheckResult{}
	if releaseVersion.Compare(currentVersion) > 0 {
		artifact, ok := findArtifact(manifest.Artifacts, runtime.GOOS, runtime.GOARCH, opts.Channel, opts.NativeCompat)
		if ok {
			native := &UpdateInfo{
				Version:      manifest.Release.Version,
				Channel:      artifact.Metadata["channel"],
				ArtifactURL:  resolveURL(opts.ManifestURL, artifact.URL),
				ArtifactHash: artifact.Checksum,
				ArtifactSize: artifact.Size,
			}
			if minVersion := strings.TrimSpace(artifact.Metadata["mandatory_min_version"]); minVersion != "" {
				mandatoryVersion, err := version.Parse(minVersion)
				if err != nil {
					return nil, fmt.Errorf("parse mandatory_min_version: %w", err)
				}
				native.Mandatory = currentVersion.Compare(mandatoryVersion) < 0
			}

			if manifest.Delta != nil && strings.TrimSpace(manifest.Delta.ManifestURL) != "" && strings.TrimSpace(opts.CurrentHash) != "" {
				deltaManifest, err := c.fetchDeltaManifest(ctx, resolveURL(opts.ManifestURL, manifest.Delta.ManifestURL))
				if err != nil {
					return nil, err
				}
				if patch, ok := findPatch(deltaManifest, artifact.Path, opts.CurrentHash, artifact.Checksum); ok {
					native.DeltaURL = resolveURL(resolveURL(opts.ManifestURL, manifest.Delta.ManifestURL), patch.Patch)
					native.DeltaHash = withSHA256Prefix(patch.PatchSHA256)
					native.DeltaSize = patch.PatchSize
					native.DeltaFromHash = withSHA256Prefix(patch.FromSHA256)
				}
			}

			result.Native = native
		}
	}

	if bundle, ok := findFrontendBundle(manifest.FrontendBundles, opts.Channel, opts.NativeCompat); ok {
		bundleVersionText := firstNonEmpty(bundle.Version, manifest.Release.Version)
		bundleVersion, err := version.Parse(bundleVersionText)
		if err != nil {
			return nil, fmt.Errorf("parse frontend bundle version: %w", err)
		}
		if bundleVersion.Compare(currentVersion) > 0 {
			result.Frontend = &FrontendUpdateInfo{
				Channel:  firstNonEmpty(bundle.Channel, opts.Channel),
				Version:  bundleVersionText,
				CompatID: bundle.CompatID,
				URL:      resolveURL(opts.ManifestURL, bundle.URL),
				Hash:     bundle.Checksum,
				Size:     bundle.Size,
			}
		}
	}

	result.Available = result.Native != nil || result.Frontend != nil
	return result, nil
}

func (c *HTTPChecker) fetchReleaseManifest(ctx context.Context, manifestURL string) (*release.Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", contract.PreferredAcceptHeader())
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch release manifest: unexpected status %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return release.DecodeManifest(data, resp.Header.Get("Content-Type"))
}

func (c *HTTPChecker) fetchDeltaManifest(ctx context.Context, manifestURL string) (*delta.PatchManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", contract.PreferredAcceptHeader())
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch delta manifest: unexpected status %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return delta.DecodeManifest(data, resp.Header.Get("Content-Type"))
}

func findArtifact(artifacts []release.ManifestArtifact, osName, arch, channel, nativeCompat string) (release.ManifestArtifact, bool) {
	for _, artifact := range artifacts {
		if artifact.OS != osName || artifact.Arch != arch {
			continue
		}
		if !matchesMetadataFilter(artifact.Metadata, "channel", channel) {
			continue
		}
		if !matchesMetadataFilter(artifact.Metadata, "native_compat", nativeCompat) && !matchesMetadataFilter(artifact.Metadata, "compat_id", nativeCompat) {
			continue
		}
		return artifact, true
	}
	return release.ManifestArtifact{}, false
}

func findFrontendBundle(bundles []release.ManifestFrontendBundle, channel, nativeCompat string) (release.ManifestFrontendBundle, bool) {
	channel = strings.TrimSpace(channel)
	nativeCompat = strings.TrimSpace(nativeCompat)

	var fallback *release.ManifestFrontendBundle
	for i := range bundles {
		bundle := bundles[i]
		if channel != "" && strings.TrimSpace(bundle.Channel) != "" && strings.TrimSpace(bundle.Channel) != channel {
			continue
		}

		bundleCompat := strings.TrimSpace(bundle.CompatID)
		switch {
		case nativeCompat == "" || bundleCompat == "" || bundleCompat == nativeCompat:
			if bundleCompat == nativeCompat && nativeCompat != "" {
				return bundle, true
			}
			if fallback == nil {
				copy := bundle
				fallback = &copy
			}
		}
	}
	if fallback == nil {
		return release.ManifestFrontendBundle{}, false
	}
	return *fallback, true
}

func matchesMetadataFilter(metadata map[string]string, key, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	if metadata == nil {
		return true
	}
	actual := strings.TrimSpace(metadata[key])
	return actual == "" || actual == expected
}

func findPatch(manifest *delta.PatchManifest, artifactPath, currentHash, targetHash string) (delta.PatchManifestEntry, bool) {
	if manifest == nil {
		return delta.PatchManifestEntry{}, false
	}
	currentHash = trimSHA256Prefix(currentHash)
	targetHash = trimSHA256Prefix(targetHash)
	for _, patch := range manifest.Patches {
		if patch.Artifact != artifactPath {
			continue
		}
		if patch.FromSHA256 != currentHash {
			continue
		}
		if patch.ToSHA256 != targetHash {
			continue
		}
		return patch, true
	}
	return delta.PatchManifestEntry{}, false
}

func resolveURL(baseURL, ref string) string {
	if strings.TrimSpace(ref) == "" {
		return ""
	}
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return ref
	}
	parsedRef, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return parsedBase.ResolveReference(parsedRef).String()
}

func withSHA256Prefix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "sha256:") {
		return value
	}
	return "sha256:" + value
}

func trimSHA256Prefix(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "sha256:")
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
