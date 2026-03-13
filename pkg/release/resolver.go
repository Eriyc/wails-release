package release

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

const (
	ProviderGitHub = "github"
	ProviderHTTP   = "http"
)

type Resolver interface {
	Provider() string
	ManifestURL(tag string) string
	DeltaManifestURL(tag string) string
	ArtifactURL(tag, assetName string) string
}

type GitHubResolver struct {
	repository string
	baseURL    string
}

func NewGitHubResolver(repository, apiBaseURL string) *GitHubResolver {
	return &GitHubResolver{
		repository: strings.TrimSpace(repository),
		baseURL:    githubDownloadBaseURL(apiBaseURL),
	}
}

func (r *GitHubResolver) Provider() string {
	return ProviderGitHub
}

func (r *GitHubResolver) ManifestURL(tag string) string {
	return r.ArtifactURL(tag, ManifestAssetName)
}

func (r *GitHubResolver) DeltaManifestURL(tag string) string {
	return r.ArtifactURL(tag, DeltaManifestAssetName)
}

func (r *GitHubResolver) ArtifactURL(tag, assetName string) string {
	if r.repository == "" || tag == "" || assetName == "" {
		return ""
	}
	escapedTag := url.PathEscape(tag)
	escapedName := url.PathEscape(assetName)
	return strings.TrimRight(r.baseURL, "/") + "/" + r.repository + "/releases/download/" + escapedTag + "/" + escapedName
}

type HTTPResolver struct {
	baseURL            string
	manifestPath       string
	deltaManifestPath  string
	downloadPathPrefix string
}

func NewHTTPResolver(baseURL, manifestPath, deltaManifestPath, downloadPathPrefix string) *HTTPResolver {
	return &HTTPResolver{
		baseURL:            strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		manifestPath:       normalizeHTTPPath(manifestPath),
		deltaManifestPath:  normalizeHTTPPath(deltaManifestPath),
		downloadPathPrefix: normalizeHTTPPath(downloadPathPrefix),
	}
}

func (r *HTTPResolver) Provider() string {
	return ProviderHTTP
}

func (r *HTTPResolver) ManifestURL(_ string) string {
	return r.baseURL + r.manifestPath
}

func (r *HTTPResolver) DeltaManifestURL(_ string) string {
	// The gateway exposes a stable delta manifest entrypoint.
	return r.baseURL + r.deltaManifestPath
}

func (r *HTTPResolver) ArtifactURL(tag, assetName string) string {
	if r.baseURL == "" || tag == "" || assetName == "" {
		return ""
	}
	return fmt.Sprintf("%s%s/%s/%s", r.baseURL, r.downloadPathPrefix, url.PathEscape(tag), url.PathEscape(assetName))
}

func normalizeHTTPPath(value string) string {
	if value == "" {
		return "/"
	}
	value = "/" + strings.TrimLeft(strings.TrimSpace(value), "/")
	return path.Clean(value)
}

func githubDownloadBaseURL(apiBaseURL string) string {
	base := strings.TrimSpace(apiBaseURL)
	if base == "" || base == "https://api.github.com" {
		return "https://github.com"
	}
	base = strings.TrimRight(base, "/")
	base = strings.TrimSuffix(base, "/api/v3")
	base = strings.TrimSuffix(base, "/api")
	if strings.Contains(base, "api.") {
		base = strings.Replace(base, "://api.", "://", 1)
	}
	return base
}
