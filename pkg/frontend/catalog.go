package frontend

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Eriyc/wailsrel/pkg/version"
)

const (
	CatalogSchemaVersion = 1
	BundleKindCodepush   = "codepush"
	BundleKindExperiment = "experiment"
)

var (
	bundleChecksumPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	bundleNamePattern     = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

type Catalog struct {
	SchemaVersion int               `json:"schema_version"`
	AppID         string            `json:"app_id"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Codepush      []CodepushEntry   `json:"codepush,omitempty"`
	Experiments   []ExperimentEntry `json:"experiments,omitempty"`
	Signature     string            `json:"signature"`
	SignedAt      time.Time         `json:"signed_at,omitempty"`
}

type CodepushEntry struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	CompatID    string    `json:"compat_id,omitempty"`
	URL         string    `json:"url"`
	Checksum    string    `json:"checksum"`
	Size        int64     `json:"size"`
	Force       bool      `json:"force,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

type ExperimentEntry struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	CompatID    string    `json:"compat_id,omitempty"`
	URL         string    `json:"url"`
	Checksum    string    `json:"checksum"`
	Size        int64     `json:"size"`
	DisplayName string    `json:"display_name,omitempty"`
	Description string    `json:"description,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

func DecodeCatalog(data []byte, publicKey, expectedAppID string) (*Catalog, error) {
	return DecodeCatalogResponse(data, "application/json", publicKey, expectedAppID)
}

func (c Catalog) Verify(publicKey, expectedAppID string) error {
	if c.SchemaVersion != CatalogSchemaVersion {
		return fmt.Errorf("unsupported frontend catalog schema version %d", c.SchemaVersion)
	}
	if expectedAppID = strings.TrimSpace(expectedAppID); expectedAppID != "" && strings.TrimSpace(c.AppID) != expectedAppID {
		return fmt.Errorf("frontend catalog app id %q does not match expected app id %q", c.AppID, expectedAppID)
	}
	if err := c.ValidateEntries(); err != nil {
		return err
	}

	payload, err := c.signedPayload()
	if err != nil {
		return err
	}
	key, err := parseCatalogPublicKey(publicKey)
	if err != nil {
		return err
	}
	signature, err := decodeCatalogSignature(c.Signature)
	if err != nil {
		return err
	}
	if !ed25519.Verify(key, payload, signature) {
		legacyPayload, legacyErr := c.legacySignedPayload()
		if legacyErr != nil || !ed25519.Verify(key, legacyPayload, signature) {
			return fmt.Errorf("frontend catalog signature verification failed")
		}
	}
	return nil
}

func (c Catalog) ValidateEntries() error {
	if strings.TrimSpace(c.AppID) == "" {
		return fmt.Errorf("frontend catalog app_id is required")
	}
	if strings.TrimSpace(c.Signature) == "" {
		return fmt.Errorf("frontend catalog signature is required")
	}
	for _, entry := range c.Codepush {
		if err := validateCatalogBundle(BundleKindCodepush, entry.Name, entry.Version, entry.CompatID, entry.URL, entry.Checksum, entry.Size, entry.PublishedAt); err != nil {
			return err
		}
	}
	for _, entry := range c.Experiments {
		if err := validateCatalogBundle(BundleKindExperiment, entry.Name, entry.Version, entry.CompatID, entry.URL, entry.Checksum, entry.Size, entry.PublishedAt); err != nil {
			return err
		}
	}
	return nil
}

func CompatibleCodepush(entries []CodepushEntry, nativeCompat string) []CodepushEntry {
	filtered := make([]CodepushEntry, 0, len(entries))
	for _, entry := range entries {
		if isCompatibleCompatID(entry.CompatID, nativeCompat) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func CompatibleExperiments(entries []ExperimentEntry, nativeCompat string) []ExperimentEntry {
	filtered := make([]ExperimentEntry, 0, len(entries))
	for _, entry := range entries {
		if isCompatibleCompatID(entry.CompatID, nativeCompat) {
			filtered = append(filtered, entry)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		left := strings.TrimSpace(filtered[i].DisplayName)
		right := strings.TrimSpace(filtered[j].DisplayName)
		if left == "" {
			left = filtered[i].Name
		}
		if right == "" {
			right = filtered[j].Name
		}
		if left == right {
			return filtered[i].Name < filtered[j].Name
		}
		return left < right
	})
	return filtered
}

func SelectNewestCodepush(entries []CodepushEntry, nativeCompat string) *CodepushEntry {
	filtered := CompatibleCodepush(entries, nativeCompat)
	if len(filtered) == 0 {
		return nil
	}
	sort.Slice(filtered, func(i, j int) bool {
		if !filtered[i].PublishedAt.Equal(filtered[j].PublishedAt) {
			return filtered[i].PublishedAt.After(filtered[j].PublishedAt)
		}
		return compareBundleVersions(filtered[i].Version, filtered[j].Version) > 0
	})
	selected := filtered[0]
	return &selected
}

func ValidateVariantName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if !bundleNamePattern.MatchString(name) {
		return fmt.Errorf("invalid name %q: only lowercase [a-z0-9-_]+ is allowed", name)
	}
	return nil
}

func ValidateBundleChecksum(checksum string) error {
	checksum = strings.TrimSpace(checksum)
	if !bundleChecksumPattern.MatchString(checksum) {
		return fmt.Errorf("invalid checksum %q: expected sha256:<64 hex>", checksum)
	}
	return nil
}

func validateCatalogBundle(kind, name, bundleVersion, compatID, rawURL, checksum string, size int64, publishedAt time.Time) error {
	if err := ValidateVariantName(name); err != nil {
		return fmt.Errorf("%s entry: %w", kind, err)
	}
	if strings.TrimSpace(bundleVersion) == "" {
		return fmt.Errorf("%s %q: version is required", kind, name)
	}
	if _, err := version.Parse(bundleVersion); err != nil {
		return fmt.Errorf("%s %q: %w", kind, name, err)
	}
	if strings.TrimSpace(compatID) == "" {
		compatID = ""
	}
	if err := ValidateBundleChecksum(checksum); err != nil {
		return fmt.Errorf("%s %q: %w", kind, name, err)
	}
	if size < 0 {
		return fmt.Errorf("%s %q: size must be non-negative", kind, name)
	}
	if publishedAt.IsZero() {
		return fmt.Errorf("%s %q: published_at is required", kind, name)
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("%s %q: invalid url: %w", kind, name, err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("%s %q: url must be https", kind, name)
	}
	return nil
}

func (c Catalog) signedPayload() ([]byte, error) {
	copy := c
	copy.Signature = ""
	return copy.canonicalSignedPayload()
}

func (c Catalog) legacySignedPayload() ([]byte, error) {
	copy := c
	copy.Signature = ""
	return json.Marshal(copy)
}

func parseCatalogPublicKey(value string) (ed25519.PublicKey, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("frontend catalog public key is required")
	}

	decode := []func(string) ([]byte, error){
		func(v string) ([]byte, error) { return base64.StdEncoding.DecodeString(v) },
		func(v string) ([]byte, error) { return base64.RawStdEncoding.DecodeString(v) },
		func(v string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(v) },
		func(v string) ([]byte, error) { return hex.DecodeString(v) },
	}
	for _, fn := range decode {
		data, err := fn(value)
		if err == nil && len(data) == ed25519.PublicKeySize {
			return ed25519.PublicKey(data), nil
		}
	}
	return nil, fmt.Errorf("frontend catalog public key must be %d-byte Ed25519 key encoded as base64 or hex", ed25519.PublicKeySize)
}

func decodeCatalogSignature(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("frontend catalog signature is required")
	}
	decode := []func(string) ([]byte, error){
		func(v string) ([]byte, error) { return base64.StdEncoding.DecodeString(v) },
		func(v string) ([]byte, error) { return base64.RawStdEncoding.DecodeString(v) },
		func(v string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(v) },
		func(v string) ([]byte, error) { return hex.DecodeString(v) },
	}
	for _, fn := range decode {
		data, err := fn(value)
		if err == nil && len(data) == ed25519.SignatureSize {
			return data, nil
		}
	}
	return nil, fmt.Errorf("frontend catalog signature must be a %d-byte Ed25519 signature encoded as base64 or hex", ed25519.SignatureSize)
}

func isCompatibleCompatID(bundleCompat, nativeCompat string) bool {
	bundleCompat = strings.TrimSpace(bundleCompat)
	nativeCompat = strings.TrimSpace(nativeCompat)
	return bundleCompat == "" || nativeCompat == "" || bundleCompat == nativeCompat
}

func compareBundleVersions(left, right string) int {
	leftVersion, leftErr := version.Parse(left)
	rightVersion, rightErr := version.Parse(right)
	switch {
	case leftErr == nil && rightErr == nil:
		return leftVersion.Compare(rightVersion)
	case leftErr == nil:
		return 1
	case rightErr == nil:
		return -1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
