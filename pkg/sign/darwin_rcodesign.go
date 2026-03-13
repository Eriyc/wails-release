package sign

import (
	"context"
	"fmt"
	"time"
)

type rcodesignSigner struct {
	baseSigner
}

func (s *rcodesignSigner) Provider() string {
	return "apple-rcodesign"
}

func (s *rcodesignSigner) Available(context.Context) error {
	if err := s.requireTools(RequiredTools(s.cfg)...); err != nil {
		return err
	}
	if err := ensureFileExists("rcodesign certificate", s.certificatePath(SignOpts{})); err != nil {
		return err
	}
	if s.cfg.Notarize {
		if err := ensureFileExists("RCODESIGN_API_KEY_FILE", s.apiKeyFile(SignOpts{})); err != nil {
			return err
		}
	}
	return nil
}

func (s *rcodesignSigner) Sign(ctx context.Context, path string, opts SignOpts) (*SignResult, error) {
	start := time.Now()
	certPath := s.certificatePath(opts)
	if err := ensureFileExists("rcodesign certificate", certPath); err != nil {
		return nil, err
	}

	passwordFile := credentialValue(opts, "p12_password_file", "", "RCODESIGN_P12_PASSWORD_FILE")
	password := credentialValue(opts, "p12_password", "", "RCODESIGN_P12_PASSWORD")

	args := []string{"sign", "--p12-file", certPath}
	if passwordFile != "" {
		args = append(args, "--p12-password-file", passwordFile)
	} else if password != "" {
		args = append(args, "--p12-password", password)
	}
	if isAppBundle(path) {
		args = append(args, "--code-signature-flags", "runtime")
	}
	args = append(args, path)
	if _, err := s.run(ctx, opts.Timeout, "rcodesign", args...); err != nil {
		return nil, err
	}

	notarized := false
	if opts.Notarize {
		apiKeyFile := s.apiKeyFile(opts)
		if err := ensureFileExists("RCODESIGN_API_KEY_FILE", apiKeyFile); err != nil {
			return nil, err
		}
		if _, err := s.run(ctx, opts.Timeout, "rcodesign", "notary-submit", "--api-key-file", apiKeyFile, "--wait", "--staple", path); err != nil {
			return nil, err
		}
		notarized = true
	}

	return signedResult(start, notarized), nil
}

func (s *rcodesignSigner) Verify(ctx context.Context, path string) error {
	if _, err := s.run(ctx, 0, "rcodesign", "print-signature-info", path); err != nil {
		return fmt.Errorf("verify rcodesign signature: %w", err)
	}
	return nil
}

func (s *rcodesignSigner) certificatePath(opts SignOpts) string {
	return credentialValue(opts, "p12_file", s.cfg.Identity, "RCODESIGN_P12_FILE")
}

func (s *rcodesignSigner) apiKeyFile(opts SignOpts) string {
	return credentialValue(opts, "api_key_file", "", "RCODESIGN_API_KEY_FILE")
}

var _ Signer = (*rcodesignSigner)(nil)
