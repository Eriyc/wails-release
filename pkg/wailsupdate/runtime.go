package wailsupdate

import (
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Eriyc/wailsrel/pkg/frontend"
)

const defaultRuntimeClientTimeout = 45 * time.Second

type RuntimeOptions struct {
	AppID          string
	CurrentVersion string
	Channel        string
	NativeCompat   string
	TargetPath     string
	TempDir        string
	EventPrefix    string
	Source         RuntimeSource
	Frontend       RuntimeFrontend
	Client         *http.Client
	ClientTimeout  time.Duration
	BearerToken    string
	DescribeState  func(*State)
}

type RuntimeSource struct {
	BaseURL     string
	Repository  string
	ManifestURL string
}

type RuntimeFrontend struct {
	Enabled          bool
	CatalogURL       string
	CatalogPublicKey string
	AutoCheck        bool
}

type Runtime struct {
	service         *Service
	frontendManager *frontend.BundleManager
}

func NewRuntime(opts RuntimeOptions) (*Runtime, error) {
	manifestURL, err := resolveRuntimeManifestURL(opts.Source)
	if err != nil {
		return nil, err
	}

	var (
		frontendManager    *frontend.BundleManager
		frontendCatalogURL string
	)
	if runtimeFrontendEnabled(opts.Frontend) {
		if strings.TrimSpace(opts.AppID) == "" {
			return nil, errRuntime("frontend app ID is required when frontend runtime is enabled")
		}
		frontendCatalogURL, err = resolveRuntimeCatalogURL(opts.Source, opts.Frontend)
		if err != nil {
			return nil, err
		}
		frontendManager = &frontend.BundleManager{
			AppID:        strings.TrimSpace(opts.AppID),
			NativeCompat: strings.TrimSpace(opts.NativeCompat),
		}
	}

	service := NewService(Options{
		ManifestURL:              manifestURL,
		CurrentVersion:           strings.TrimSpace(opts.CurrentVersion),
		Channel:                  strings.TrimSpace(opts.Channel),
		NativeCompat:             strings.TrimSpace(opts.NativeCompat),
		FrontendCatalogURL:       frontendCatalogURL,
		FrontendCatalogPublicKey: strings.TrimSpace(opts.Frontend.CatalogPublicKey),
		FrontendAutoCheck:        opts.Frontend.AutoCheck,
		TargetPath:               strings.TrimSpace(opts.TargetPath),
		TempDir:                  strings.TrimSpace(opts.TempDir),
		Client:                   runtimeHTTPClient(opts.Client, opts.ClientTimeout, opts.BearerToken),
		FrontendManager:          frontendManager,
		EventPrefix:              strings.TrimSpace(opts.EventPrefix),
		DescribeState:            opts.DescribeState,
	})

	return &Runtime{
		service:         service,
		frontendManager: frontendManager,
	}, nil
}

func ManifestURLFromBaseURL(baseURL string) string {
	return joinRuntimeURLPath(baseURL, "/manifest")
}

func FrontendCatalogURLFromBaseURL(baseURL string) string {
	return joinRuntimeURLPath(baseURL, "/frontend/catalog")
}

func (r *Runtime) Service() *Service {
	if r == nil {
		return nil
	}
	return r.service
}

func (r *Runtime) FrontendManager() *frontend.BundleManager {
	if r == nil {
		return nil
	}
	return r.frontendManager
}

func (r *Runtime) AssetFS(embedded fs.FS) fs.FS {
	if r == nil {
		return frontend.NewRuntimeFS(nil, embedded)
	}
	return frontend.NewRuntimeFS(r.frontendManager, embedded)
}

func resolveRuntimeManifestURL(source RuntimeSource) (string, error) {
	baseURL := strings.TrimSpace(source.BaseURL)
	repository := strings.TrimSpace(source.Repository)
	manifestURL := strings.TrimSpace(source.ManifestURL)

	if baseURL != "" && repository != "" {
		return "", errRuntime("base URL and repository cannot both be set")
	}
	if manifestURL != "" {
		return manifestURL, nil
	}
	if baseURL != "" {
		return ManifestURLFromBaseURL(baseURL), nil
	}
	if repository != "" {
		return GitHubLatestManifestURL(repository), nil
	}
	return "", errRuntime("manifest URL, base URL, or repository is required")
}

func resolveRuntimeCatalogURL(source RuntimeSource, frontendOpts RuntimeFrontend) (string, error) {
	if catalogURL := strings.TrimSpace(frontendOpts.CatalogURL); catalogURL != "" {
		return catalogURL, nil
	}
	if baseURL := strings.TrimSpace(source.BaseURL); baseURL != "" {
		return FrontendCatalogURLFromBaseURL(baseURL), nil
	}
	return "", errRuntime("frontend catalog URL is required when frontend runtime is enabled without a base URL")
}

func runtimeFrontendEnabled(opts RuntimeFrontend) bool {
	return opts.Enabled ||
		strings.TrimSpace(opts.CatalogURL) != "" ||
		strings.TrimSpace(opts.CatalogPublicKey) != ""
}

func runtimeHTTPClient(base *http.Client, timeout time.Duration, bearerToken string) *http.Client {
	client := base
	if client == nil {
		if timeout <= 0 {
			timeout = defaultRuntimeClientTimeout
		}
		client = &http.Client{Timeout: timeout}
	} else {
		cloned := *client
		client = &cloned
	}
	if strings.TrimSpace(bearerToken) != "" {
		client.Transport = BearerTransport(client.Transport, bearerToken)
	}
	return client
}

func joinRuntimeURLPath(baseURL, suffix string) string {
	baseURL = strings.TrimSpace(baseURL)
	suffix = strings.TrimSpace(suffix)
	if baseURL == "" || suffix == "" {
		return ""
	}

	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(baseURL, "/") + suffix
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/") + suffix
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func errRuntime(message string) error {
	return &runtimeError{message: message}
}

type runtimeError struct {
	message string
}

func (e *runtimeError) Error() string {
	return e.message
}
