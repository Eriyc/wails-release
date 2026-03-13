package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	releasepkg "github.com/Eriyc/wailsrel/pkg/release"
)

type Server struct {
	cfg      Config
	github   *releasepkg.GitHubClient
	verifier *JWTVerifier
	cache    *releaseCache
	mux      *http.ServeMux
}

type releaseCache struct {
	ttl    time.Duration
	mu     sync.Mutex
	latest cachedRelease
	byTag  map[string]cachedRelease
}

type cachedRelease struct {
	value   *releasepkg.GitHubRelease
	expires time.Time
}

func NewServer(cfg Config) (*Server, error) {
	verifier := NewJWTVerifier(cfg.JWKSURL, cfg.Issuer, cfg.Audience, nil)
	server := &Server{
		cfg:      cfg,
		github:   releasepkg.NewGitHubClient(cfg.APIBaseURL, cfg.Token, nil),
		verifier: verifier,
		cache: &releaseCache{
			ttl:   60 * time.Second,
			byTag: map[string]cachedRelease{},
		},
		mux: http.NewServeMux(),
	}
	server.routes()
	return server, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := s.verifier.VerifyRequest(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /manifest.json", func(w http.ResponseWriter, r *http.Request) {
		s.serveLatestAsset(w, r, releasepkg.ManifestAssetName)
	})
	s.mux.HandleFunc("GET /delta/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		s.serveLatestAsset(w, r, releasepkg.DeltaManifestAssetName)
	})
	s.mux.HandleFunc("GET /download/{tag}/{asset_name}", s.serveTaggedAsset)
}

func (s *Server) serveLatestAsset(w http.ResponseWriter, r *http.Request, assetName string) {
	release, err := s.latestRelease(r.Context())
	if err != nil {
		http.Error(w, err.Error(), statusForError(err))
		return
	}
	asset, err := findReleaseAsset(release, assetName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := s.streamAsset(w, r, asset); err != nil {
		http.Error(w, err.Error(), statusForError(err))
	}
}

func (s *Server) serveTaggedAsset(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.PathValue("tag"))
	assetName := strings.TrimSpace(r.PathValue("asset_name"))
	release, err := s.releaseByTag(r.Context(), tag)
	if err != nil {
		http.Error(w, err.Error(), statusForError(err))
		return
	}
	asset, err := findReleaseAsset(release, assetName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := s.streamAsset(w, r, asset); err != nil {
		http.Error(w, err.Error(), statusForError(err))
	}
}

func (s *Server) streamAsset(w http.ResponseWriter, r *http.Request, asset releasepkg.GitHubAsset) error {
	resp, err := s.github.StreamAsset(r.Context(), asset.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header, "Content-Type")
	copyHeader(w.Header(), resp.Header, "Content-Length")
	copyHeader(w.Header(), resp.Header, "Content-Disposition")
	copyHeader(w.Header(), resp.Header, "ETag")
	copyHeader(w.Header(), resp.Header, "Last-Modified")
	copyHeader(w.Header(), resp.Header, "Cache-Control")
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}

func (s *Server) latestRelease(ctx context.Context) (*releasepkg.GitHubRelease, error) {
	return s.cache.get("latest", func() (*releasepkg.GitHubRelease, error) {
		return s.github.GetLatestRelease(ctx, s.cfg.Repository)
	})
}

func (s *Server) releaseByTag(ctx context.Context, tag string) (*releasepkg.GitHubRelease, error) {
	return s.cache.get(tag, func() (*releasepkg.GitHubRelease, error) {
		return s.github.GetReleaseByTag(ctx, s.cfg.Repository, tag)
	})
}

func (c *releaseCache) get(key string, fetch func() (*releasepkg.GitHubRelease, error)) (*releasepkg.GitHubRelease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var current cachedRelease
	if key == "latest" {
		current = c.latest
	} else {
		current = c.byTag[key]
	}
	if current.value != nil && time.Now().Before(current.expires) {
		return current.value, nil
	}

	value, err := fetch()
	if err != nil {
		return nil, err
	}
	cached := cachedRelease{value: value, expires: time.Now().Add(c.ttl)}
	if key == "latest" {
		c.latest = cached
	} else {
		c.byTag[key] = cached
	}
	return value, nil
}

func findReleaseAsset(release *releasepkg.GitHubRelease, assetName string) (releasepkg.GitHubAsset, error) {
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			return asset, nil
		}
	}
	return releasepkg.GitHubAsset{}, fmt.Errorf("asset %s not found", assetName)
}

func statusForError(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case strings.Contains(err.Error(), "not found"):
		return http.StatusNotFound
	default:
		return http.StatusBadGateway
	}
}

func copyHeader(dst, src http.Header, key string) {
	if value := src.Get(key); value != "" {
		dst.Set(key, value)
	}
}
