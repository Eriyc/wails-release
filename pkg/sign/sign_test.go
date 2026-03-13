package sign

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	internalexec "github.com/you/wailsrel/internal/exec"
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

func TestAppleSignerSignAndVerify(t *testing.T) {
	toolsDir := t.TempDir()
	writeExecutable(t, filepath.Join(toolsDir, "codesign"))
	writeExecutable(t, filepath.Join(toolsDir, "xcrun"))
	writeExecutable(t, filepath.Join(toolsDir, "spctl"))
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	runner := &recordingRunner{}
	signer, err := NewSignerWithOptions(config.SignConfig{
		Provider: "apple",
		Identity: "Developer ID Application: Example (TEAMID)",
		Notarize: true,
		AppleID:  "dev@example.com",
		Password: "app-password",
		TeamID:   "TEAMID",
	}, Options{Runner: runner})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	if err := signer.Available(context.Background()); err != nil {
		t.Fatalf("available: %v", err)
	}

	appPath := filepath.Join(t.TempDir(), "MyApp.app")
	if err := os.MkdirAll(appPath, 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	if _, err := signer.Sign(context.Background(), appPath, SignOpts{
		Identity: "Developer ID Application: Example (TEAMID)",
		Timeout:  time.Second,
	}); err != nil {
		t.Fatalf("sign app: %v", err)
	}
	if err := signer.Verify(context.Background(), appPath); err != nil {
		t.Fatalf("verify app: %v", err)
	}

	dmgPath := filepath.Join(t.TempDir(), "MyApp.dmg")
	if err := os.WriteFile(dmgPath, []byte("dmg"), 0o644); err != nil {
		t.Fatalf("write dmg: %v", err)
	}
	result, err := signer.Sign(context.Background(), dmgPath, SignOpts{
		Identity: "Developer ID Application: Example (TEAMID)",
		Notarize: true,
		Timeout:  time.Second,
		Credentials: map[string]string{
			"apple_id": "dev@example.com",
			"password": "app-password",
			"team_id":  "TEAMID",
		},
	})
	if err != nil {
		t.Fatalf("sign dmg: %v", err)
	}
	if !result.Notarized {
		t.Fatal("expected dmg notarization")
	}
	if err := signer.Verify(context.Background(), dmgPath); err != nil {
		t.Fatalf("verify dmg: %v", err)
	}

	assertCallContains(t, runner.calls, "codesign --force --timestamp --sign Developer ID Application: Example (TEAMID) --deep --options runtime "+appPath)
	assertCallContains(t, runner.calls, "codesign --verify --deep --strict --verbose=2 "+appPath)
	assertCallContains(t, runner.calls, "spctl --assess --type exec --verbose=2 "+appPath)
	assertCallContains(t, runner.calls, "xcrun notarytool submit "+dmgPath+" --apple-id dev@example.com --password app-password --team-id TEAMID --wait")
	assertCallContains(t, runner.calls, "xcrun stapler staple "+dmgPath)
	assertCallContains(t, runner.calls, "spctl --assess --type open --verbose=2 "+dmgPath)
}

func TestRCodesignSignerSignAndVerify(t *testing.T) {
	toolsDir := t.TempDir()
	writeExecutable(t, filepath.Join(toolsDir, "rcodesign"))
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	certPath := filepath.Join(t.TempDir(), "cert.p12")
	apiKeyPath := filepath.Join(t.TempDir(), "AuthKey.p8")
	if err := os.WriteFile(certPath, []byte("p12"), 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(apiKeyPath, []byte("key"), 0o644); err != nil {
		t.Fatalf("write api key: %v", err)
	}

	runner := &recordingRunner{}
	signer, err := NewSignerWithOptions(config.SignConfig{
		Provider: "apple-rcodesign",
		Identity: certPath,
		Notarize: true,
	}, Options{Runner: runner})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	t.Setenv("RCODESIGN_API_KEY_FILE", apiKeyPath)
	if err := signer.Available(context.Background()); err != nil {
		t.Fatalf("available: %v", err)
	}

	appPath := filepath.Join(t.TempDir(), "MyApp.app")
	if err := os.MkdirAll(appPath, 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	result, err := signer.Sign(context.Background(), appPath, SignOpts{
		Notarize: true,
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("sign app: %v", err)
	}
	if !result.Notarized {
		t.Fatal("expected rcodesign notarization")
	}
	if err := signer.Verify(context.Background(), appPath); err != nil {
		t.Fatalf("verify app: %v", err)
	}

	assertCallContains(t, runner.calls, "rcodesign sign --p12-file "+certPath+" --code-signature-flags runtime "+appPath)
	assertCallContains(t, runner.calls, "rcodesign notary-submit --api-key-file "+apiKeyPath+" --wait --staple "+appPath)
	assertCallContains(t, runner.calls, "rcodesign print-signature-info "+appPath)
}

func TestAzureSignerSignAndVerify(t *testing.T) {
	toolsDir := t.TempDir()
	writeExecutable(t, filepath.Join(toolsDir, "signtool"))
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dlibPath := filepath.Join(t.TempDir(), "Azure.CodeSigning.Dlib.dll")
	if err := os.WriteFile(dlibPath, []byte("dlib"), 0o644); err != nil {
		t.Fatalf("write dlib: %v", err)
	}

	t.Setenv("AZURE_TENANT_ID", "tenant")
	t.Setenv("AZURE_CLIENT_ID", "client")
	t.Setenv("AZURE_CLIENT_SECRET", "secret")
	t.Setenv("AZURE_TRUSTED_SIGNING_DLIB", dlibPath)

	runner := &recordingRunner{}
	signer, err := NewSignerWithOptions(config.SignConfig{
		Provider: "azure",
		Endpoint: "https://account.codesigning.azure.net",
		Account:  "account",
		Profile:  "profile",
	}, Options{Runner: runner})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	if err := signer.Available(context.Background()); err != nil {
		t.Fatalf("available: %v", err)
	}

	exePath := filepath.Join(t.TempDir(), "MyApp.exe")
	if err := os.WriteFile(exePath, []byte("exe"), 0o644); err != nil {
		t.Fatalf("write exe: %v", err)
	}

	if _, err := signer.Sign(context.Background(), exePath, SignOpts{Timeout: time.Second}); err != nil {
		t.Fatalf("sign exe: %v", err)
	}
	if err := signer.Verify(context.Background(), exePath); err != nil {
		t.Fatalf("verify exe: %v", err)
	}

	assertCallContains(t, runner.calls, "signtool sign /v /debug /fd SHA256 /tr http://timestamp.acs.microsoft.com /td SHA256 /dlib "+dlibPath)
	assertCallContains(t, runner.calls, "signtool verify /pa /v "+exePath)
}

func TestAzureSignerAvailableRequiresCredentials(t *testing.T) {
	toolsDir := t.TempDir()
	writeExecutable(t, filepath.Join(toolsDir, "signtool"))
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dlibPath := filepath.Join(t.TempDir(), "Azure.CodeSigning.Dlib.dll")
	if err := os.WriteFile(dlibPath, []byte("dlib"), 0o644); err != nil {
		t.Fatalf("write dlib: %v", err)
	}
	t.Setenv("AZURE_TRUSTED_SIGNING_DLIB", dlibPath)
	t.Setenv("AZURE_TENANT_ID", "")
	t.Setenv("AZURE_CLIENT_ID", "")
	t.Setenv("AZURE_CLIENT_SECRET", "")

	signer, err := NewSigner(config.SignConfig{
		Provider: "azure",
		Endpoint: "https://account.codesigning.azure.net",
		Account:  "account",
		Profile:  "profile",
	})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	err = signer.Available(context.Background())
	if err == nil {
		t.Fatal("expected credential validation error")
	}
	for _, expected := range []string{"AZURE_TENANT_ID is required", "AZURE_CLIENT_ID is required", "AZURE_CLIENT_SECRET is required"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected %q in error, got %v", expected, err)
		}
	}
}

type recordingRunner struct {
	calls    []string
	failures map[string]error
}

func (r *recordingRunner) Run(_ context.Context, name string, args []string, _ internalexec.Options) (*internalexec.Result, error) {
	call := strings.TrimSpace(strings.Join(append([]string{name}, args...), " "))
	r.calls = append(r.calls, call)
	if err := r.failures[call]; err != nil {
		return &internalexec.Result{Stderr: err.Error(), ExitCode: 1}, errors.New("command failed")
	}
	return &internalexec.Result{}, nil
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func assertCallContains(t *testing.T, calls []string, want string) {
	t.Helper()
	for _, call := range calls {
		if strings.Contains(call, want) {
			return
		}
	}
	t.Fatalf("expected call containing %q, got %v", want, calls)
}
