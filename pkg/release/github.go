package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrReleaseNotFound = errors.New("github release not found")

type GitHubClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type GitHubRelease struct {
	ID        int64         `json:"id"`
	TagName   string        `json:"tag_name"`
	Draft     bool          `json:"draft"`
	Name      string        `json:"name"`
	UploadURL string        `json:"upload_url"`
	Assets    []GitHubAsset `json:"assets"`
}

type GitHubAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Label              string `json:"label"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func NewGitHubClient(baseURL, token string, httpClient *http.Client) *GitHubClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.github.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GitHubClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      strings.TrimSpace(token),
		httpClient: httpClient,
	}
}

func (c *GitHubClient) GetReleaseByTag(ctx context.Context, repository, tag string) (*GitHubRelease, error) {
	var release GitHubRelease
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/releases/tags/%s", c.baseURL, repository, url.PathEscape(tag)), nil, &release)
	if err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, ErrReleaseNotFound
		}
		return nil, err
	}
	return &release, nil
}

func (c *GitHubClient) GetLatestRelease(ctx context.Context, repository string) (*GitHubRelease, error) {
	var release GitHubRelease
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/releases/latest", c.baseURL, repository), nil, &release)
	if err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, ErrReleaseNotFound
		}
		return nil, err
	}
	return &release, nil
}

func (c *GitHubClient) ListReleases(ctx context.Context, repository string, perPage int) ([]GitHubRelease, error) {
	if perPage <= 0 {
		perPage = 20
	}
	var releases []GitHubRelease
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/releases?per_page=%d", c.baseURL, repository, perPage), nil, &releases)
	return releases, err
}

func (c *GitHubClient) EnsureRelease(ctx context.Context, repository, tag string) (*GitHubRelease, error) {
	release, err := c.GetReleaseByTag(ctx, repository, tag)
	if err == nil {
		return release, nil
	}
	if !errors.Is(err, ErrReleaseNotFound) {
		return nil, err
	}

	body := map[string]any{
		"tag_name": tag,
		"name":     tag,
		"draft":    false,
	}

	var created GitHubRelease
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("%s/repos/%s/releases", c.baseURL, repository), body, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (c *GitHubClient) ReplaceAssets(ctx context.Context, repository, tag string, uploads []UploadAsset) (*GitHubRelease, error) {
	release, err := c.EnsureRelease(ctx, repository, tag)
	if err != nil {
		return nil, err
	}

	existing := make(map[string]GitHubAsset, len(release.Assets))
	for _, asset := range release.Assets {
		existing[asset.Name] = asset
	}
	for _, upload := range uploads {
		if current, ok := existing[upload.Name]; ok {
			if err := c.deleteAsset(ctx, repository, current.ID); err != nil {
				return nil, err
			}
		}
		if err := c.uploadAsset(ctx, release.UploadURL, upload); err != nil {
			return nil, err
		}
	}

	return c.GetReleaseByTag(ctx, repository, tag)
}

func (c *GitHubClient) StreamAssetByName(ctx context.Context, repository, tag, assetName string) (*http.Response, error) {
	release, err := c.GetReleaseByTag(ctx, repository, tag)
	if err != nil {
		return nil, err
	}
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			return c.StreamAsset(ctx, asset.URL)
		}
	}
	return nil, fmt.Errorf("asset %s not found in release %s", assetName, tag)
}

func (c *GitHubClient) StreamAsset(ctx context.Context, assetURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("github asset download %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c *GitHubClient) uploadAsset(ctx context.Context, uploadURL string, upload UploadAsset) error {
	file, err := os.Open(upload.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	target, err := expandUploadURL(uploadURL, upload.Name)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, file)
	if err != nil {
		return err
	}
	c.applyHeaders(req)
	req.Header.Set("Content-Type", contentType(upload))
	req.ContentLength = info.Size()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github upload %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *GitHubClient) deleteAsset(ctx context.Context, repository string, assetID int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/repos/%s/releases/assets/%d", c.baseURL, repository, assetID), nil)
	if err != nil {
		return err
	}
	c.applyHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github delete asset %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *GitHubClient) doJSON(ctx context.Context, method, requestURL string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return err
	}
	c.applyHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &apiError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *GitHubClient) applyHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wailsrel")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func expandUploadURL(rawURL, assetName string) (string, error) {
	trimmed := strings.TrimSuffix(rawURL, "{?name,label}")
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("name", assetName)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func contentType(upload UploadAsset) string {
	if upload.ContentType != "" {
		return upload.ContentType
	}
	if ext := filepath.Ext(upload.Name); ext != "" {
		if value := mime.TypeByExtension(ext); value != "" {
			return value
		}
	}
	return "application/octet-stream"
}

type apiError struct {
	StatusCode int
	Message    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("github API %d: %s", e.StatusCode, e.Message)
}
