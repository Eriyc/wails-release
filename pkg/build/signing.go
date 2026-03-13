package build

import (
	"context"
	"fmt"
	"time"

	"github.com/Eriyc/wailsrel/pkg/sign"
)

func (b *WailsBuilder) signArtifact(ctx context.Context, target Target, path string, notarize bool) error {
	signer, err := b.newSigner(target.Sign)
	if err != nil {
		return err
	}

	result, err := signer.Sign(ctx, path, sign.SignOpts{
		Identity: target.Sign.Identity,
		Notarize: notarize,
		Timeout:  b.timeout,
		Credentials: map[string]string{
			"identity": target.Sign.Identity,
			"apple_id": target.Sign.AppleID,
			"password": target.Sign.Password,
			"team_id":  target.Sign.TeamID,
			"endpoint": target.Sign.Endpoint,
			"account":  target.Sign.Account,
			"profile":  target.Sign.Profile,
		},
	})
	if err != nil {
		return fmt.Errorf("sign %s with %s: %w", path, signer.Provider(), err)
	}
	if !result.Signed {
		return nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, b.timeout+time.Minute)
	defer cancel()

	if err := signer.Verify(verifyCtx, path); err != nil {
		return fmt.Errorf("verify %s with %s: %w", path, signer.Provider(), err)
	}

	return nil
}
