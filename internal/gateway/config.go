package gateway

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Addr       string
	Repository string
	Token      string
	APIBaseURL string
	JWKSURL    string
	Issuer     string
	Audience   string
}

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		Addr:       firstNonEmpty(os.Getenv("WAILSREL_GATEWAY_ADDR"), ":8080"),
		Repository: strings.TrimSpace(os.Getenv("WAILSREL_GATEWAY_GITHUB_REPOSITORY")),
		Token:      strings.TrimSpace(os.Getenv("WAILSREL_GATEWAY_GITHUB_TOKEN")),
		APIBaseURL: firstNonEmpty(os.Getenv("WAILSREL_GATEWAY_GITHUB_API_BASE_URL"), "https://api.github.com"),
		JWKSURL:    strings.TrimSpace(os.Getenv("WAILSREL_GATEWAY_JWKS_URL")),
		Issuer:     strings.TrimSpace(os.Getenv("WAILSREL_GATEWAY_JWT_ISSUER")),
		Audience:   strings.TrimSpace(os.Getenv("WAILSREL_GATEWAY_JWT_AUDIENCE")),
	}

	switch {
	case cfg.Repository == "":
		return Config{}, fmt.Errorf("WAILSREL_GATEWAY_GITHUB_REPOSITORY is required")
	case cfg.Token == "":
		return Config{}, fmt.Errorf("WAILSREL_GATEWAY_GITHUB_TOKEN is required")
	case cfg.JWKSURL == "":
		return Config{}, fmt.Errorf("WAILSREL_GATEWAY_JWKS_URL is required")
	case cfg.Issuer == "":
		return Config{}, fmt.Errorf("WAILSREL_GATEWAY_JWT_ISSUER is required")
	case cfg.Audience == "":
		return Config{}, fmt.Errorf("WAILSREL_GATEWAY_JWT_AUDIENCE is required")
	default:
		return cfg, nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
