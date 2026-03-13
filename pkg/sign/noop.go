package sign

import (
	"context"
	"time"
)

type noopSigner struct {
	baseSigner
}

func (s *noopSigner) Provider() string {
	return "none"
}

func (s *noopSigner) Available(context.Context) error {
	return nil
}

func (s *noopSigner) Sign(context.Context, string, SignOpts) (*SignResult, error) {
	return &SignResult{
		Signed:   false,
		Duration: 0,
	}, nil
}

func (s *noopSigner) Verify(context.Context, string) error {
	return nil
}

var _ Signer = (*noopSigner)(nil)

func signedResult(start time.Time, notarized bool) *SignResult {
	return &SignResult{
		Signed:    true,
		Notarized: notarized,
		Duration:  time.Since(start),
	}
}
