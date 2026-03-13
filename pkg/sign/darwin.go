package sign

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

type appleSigner struct {
	baseSigner
}

func (s *appleSigner) Provider() string {
	return "apple"
}

func (s *appleSigner) Available(context.Context) error {
	if err := s.requireTools(RequiredTools(s.cfg)...); err != nil {
		return err
	}
	if err := requireValue("sign.identity", s.cfg.Identity); err != nil {
		return err
	}
	if s.cfg.Notarize {
		return combineErrors(
			requireValue("sign.apple_id", s.cfg.AppleID),
			requireValue("sign.password", s.cfg.Password),
			requireValue("sign.team_id", s.cfg.TeamID),
		)
	}
	return nil
}

func (s *appleSigner) Sign(ctx context.Context, path string, opts SignOpts) (*SignResult, error) {
	start := time.Now()
	identity := credentialValue(opts, "identity", opts.Identity)
	if identity == "" {
		identity = s.cfg.Identity
	}
	if err := requireValue("sign.identity", identity); err != nil {
		return nil, err
	}

	args := []string{"--force", "--timestamp", "--sign", identity}
	if isAppBundle(path) {
		args = append(args, "--deep", "--options", "runtime")
	}
	args = append(args, path)
	if _, err := s.run(ctx, opts.Timeout, "codesign", args...); err != nil {
		return nil, err
	}

	notarized := false
	if opts.Notarize {
		if !isDMG(path) {
			return nil, fmt.Errorf("apple notarization currently requires a .dmg artifact: %s", path)
		}

		appleID := credentialValue(opts, "apple_id", s.cfg.AppleID, "APPLE_ID")
		password := credentialValue(opts, "password", s.cfg.Password, "APPLE_APP_PASSWORD")
		teamID := credentialValue(opts, "team_id", s.cfg.TeamID, "APPLE_TEAM_ID")
		if err := combineErrors(
			requireValue("sign.apple_id", appleID),
			requireValue("sign.password", password),
			requireValue("sign.team_id", teamID),
		); err != nil {
			return nil, err
		}

		if _, err := s.run(ctx, opts.Timeout, "xcrun", "notarytool", "submit", path, "--apple-id", appleID, "--password", password, "--team-id", teamID, "--wait"); err != nil {
			return nil, err
		}
		if _, err := s.run(ctx, opts.Timeout, "xcrun", "stapler", "staple", path); err != nil {
			return nil, err
		}
		notarized = true
	}

	return signedResult(start, notarized), nil
}

func (s *appleSigner) Verify(ctx context.Context, path string) error {
	args := []string{"--verify", "--verbose=2", path}
	if isAppBundle(path) {
		args = []string{"--verify", "--deep", "--strict", "--verbose=2", path}
	}
	if _, err := s.run(ctx, 0, "codesign", args...); err != nil {
		return err
	}

	if _, err := exec.LookPath("spctl"); err == nil {
		assessType := "exec"
		if isDMG(path) {
			assessType = "open"
		}
		if _, err := s.run(ctx, 0, "spctl", "--assess", "--type", assessType, "--verbose=2", path); err != nil {
			return err
		}
	}

	return nil
}

var _ Signer = (*appleSigner)(nil)
