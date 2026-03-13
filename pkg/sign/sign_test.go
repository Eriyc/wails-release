package sign

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/you/wailsrel/pkg/config"
)

func TestNewSignerDispatchesProviders(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.SignConfig
		provider string
	}{
		{name: "default noop", cfg: config.SignConfig{}, provider: "none"},
		{name: "apple", cfg: config.SignConfig{Provider: "apple"}, provider: "apple"},
		{name: "rcodesign", cfg: config.SignConfig{Provider: "apple-rcodesign"}, provider: "apple-rcodesign"},
		{name: "azure", cfg: config.SignConfig{Provider: "azure"}, provider: "azure"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			signer, err := NewSigner(tc.cfg)
			if err != nil {
				t.Fatalf("new signer: %v", err)
			}
			if signer.Provider() != tc.provider {
				t.Fatalf("expected provider %q, got %q", tc.provider, signer.Provider())
			}
		})
	}
}

func TestNewSignerRejectsUnknownProvider(t *testing.T) {
	if _, err := NewSigner(config.SignConfig{Provider: "mystery"}); err == nil {
		t.Fatal("expected unknown provider error")
	}
}

func TestNoopSignerSkipsSigning(t *testing.T) {
	signer, err := NewSigner(config.SignConfig{})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	result, err := signer.Sign(context.Background(), "artifact.bin", SignOpts{})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if result.Signed {
		t.Fatal("expected noop signer to skip signing")
	}
}

func TestWriteTrustedSigningMetadata(t *testing.T) {
	path, err := writeTrustedSigningMetadata("https://endpoint", "account", "profile")
	if err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if payload["Endpoint"] != "https://endpoint" {
		t.Fatalf("unexpected endpoint %q", payload["Endpoint"])
	}
	if payload["CodeSigningAccountName"] != "account" {
		t.Fatalf("unexpected account %q", payload["CodeSigningAccountName"])
	}
	if payload["CertificateProfileName"] != "profile" {
		t.Fatalf("unexpected profile %q", payload["CertificateProfileName"])
	}
}
