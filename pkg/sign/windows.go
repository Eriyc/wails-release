package sign

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type azureSigner struct {
	baseSigner
}

func (s *azureSigner) Provider() string {
	return "azure"
}

func (s *azureSigner) Available(context.Context) error {
	if err := s.requireTools(RequiredTools(s.cfg)...); err != nil {
		return err
	}
	if err := combineErrors(
		requireValue("sign.endpoint", s.cfg.Endpoint),
		requireValue("sign.account", s.cfg.Account),
		requireValue("sign.profile", s.cfg.Profile),
		requireValue("AZURE_TENANT_ID", os.Getenv("AZURE_TENANT_ID")),
		requireValue("AZURE_CLIENT_ID", os.Getenv("AZURE_CLIENT_ID")),
		requireValue("AZURE_CLIENT_SECRET", os.Getenv("AZURE_CLIENT_SECRET")),
	); err != nil {
		return err
	}
	if _, err := os.Stat(resolveTrustedSigningDlib(SignOpts{})); err != nil {
		return fmt.Errorf("trusted signing dlib: %w", err)
	}
	return nil
}

func (s *azureSigner) Sign(ctx context.Context, path string, opts SignOpts) (*SignResult, error) {
	start := time.Now()

	endpoint := credentialValue(opts, "endpoint", s.cfg.Endpoint, "AZURE_ENDPOINT")
	account := credentialValue(opts, "account", s.cfg.Account, "AZURE_CODE_SIGNING_NAME")
	profile := credentialValue(opts, "profile", s.cfg.Profile, "AZURE_CERT_PROFILE")
	if err := combineErrors(
		requireValue("sign.endpoint", endpoint),
		requireValue("sign.account", account),
		requireValue("sign.profile", profile),
	); err != nil {
		return nil, err
	}

	dlibPath := resolveTrustedSigningDlib(opts)
	if err := ensureFileExists("trusted signing dlib", dlibPath); err != nil {
		return nil, err
	}

	metaPath, err := writeTrustedSigningMetadata(endpoint, account, profile)
	if err != nil {
		return nil, err
	}
	defer os.Remove(metaPath)

	if _, err := s.run(
		ctx,
		opts.Timeout,
		"signtool",
		"sign",
		"/v",
		"/debug",
		"/fd",
		"SHA256",
		"/tr",
		"http://timestamp.acs.microsoft.com",
		"/td",
		"SHA256",
		"/dlib",
		dlibPath,
		"/dmdf",
		metaPath,
		path,
	); err != nil {
		return nil, err
	}

	return signedResult(start, false), nil
}

func (s *azureSigner) Verify(ctx context.Context, path string) error {
	if _, err := s.run(ctx, 0, "signtool", "verify", "/pa", "/v", path); err != nil {
		return fmt.Errorf("verify windows signature: %w", err)
	}
	return nil
}

func resolveTrustedSigningDlib(opts SignOpts) string {
	if value := credentialValue(opts, "dlib_path", "", "AZURE_TRUSTED_SIGNING_DLIB", "TRUSTED_SIGNING_DLIB"); value != "" {
		return value
	}

	var candidates []string
	if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
		candidates = append(candidates,
			filepath.Join(programFiles, "Microsoft Trusted Signing Client Tools", "x64", "Azure.CodeSigning.Dlib.dll"),
			filepath.Join(programFiles, "Microsoft Trusted Signing Client Tools", "x86", "Azure.CodeSigning.Dlib.dll"),
		)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	if len(candidates) > 0 {
		return candidates[0]
	}

	return ""
}

func writeTrustedSigningMetadata(endpoint, account, profile string) (string, error) {
	type metadata struct {
		Endpoint               string `json:"Endpoint"`
		CodeSigningAccountName string `json:"CodeSigningAccountName"`
		CertificateProfileName string `json:"CertificateProfileName"`
	}

	file, err := os.CreateTemp("", "wailsrel-trusted-signing-*.json")
	if err != nil {
		return "", err
	}
	defer file.Close()

	payload := metadata{
		Endpoint:               endpoint,
		CodeSigningAccountName: account,
		CertificateProfileName: profile,
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return "", err
	}

	return file.Name(), nil
}

var _ Signer = (*azureSigner)(nil)
