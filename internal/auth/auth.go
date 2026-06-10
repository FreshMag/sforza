// Package auth authenticates incoming HTTP requests.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// HeaderUserSub carries the caller's subject; with OIDC enabled it must
// match the validated JWT sub claim.
const HeaderUserSub = "X-User-Sub"

// ErrUnauthenticated is returned for any authentication failure.
var ErrUnauthenticated = errors.New("unauthenticated")

// Authenticator extracts and verifies the caller identity of a request.
type Authenticator interface {
	// Authenticate returns the verified subject or ErrUnauthenticated.
	Authenticate(r *http.Request) (string, error)
}

// Static is the development/test authenticator used when auth is disabled.
// It trusts X-User-Sub when present and falls back to the configured
// default subject.
type Static struct {
	DefaultSub string
}

// Authenticate implements Authenticator.
func (s Static) Authenticate(r *http.Request) (string, error) {
	if sub := r.Header.Get(HeaderUserSub); sub != "" {
		return sub, nil
	}
	if s.DefaultSub == "" {
		return "", fmt.Errorf("%w: no subject", ErrUnauthenticated)
	}
	return s.DefaultSub, nil
}

// OIDC validates RFC 6750 bearer tokens against an OIDC provider (Keycloak
// or any other compliant issuer) and enforces JWT.sub == X-User-Sub.
type OIDC struct {
	verifier *oidc.IDTokenVerifier
}

// NewOIDC discovers the provider configuration from the issuer URL. When
// audience is empty the audience check is skipped.
func NewOIDC(ctx context.Context, issuer, audience string) (*OIDC, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider %q: %w", issuer, err)
	}
	cfg := &oidc.Config{ClientID: audience}
	if audience == "" {
		cfg.SkipClientIDCheck = true
	}
	return &OIDC{verifier: provider.Verifier(cfg)}, nil
}

// Authenticate implements Authenticator.
func (o *OIDC) Authenticate(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	rawToken, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || rawToken == "" {
		return "", fmt.Errorf("%w: missing bearer token", ErrUnauthenticated)
	}
	token, err := o.verifier.Verify(r.Context(), rawToken)
	if err != nil {
		return "", fmt.Errorf("%w: invalid token: %v", ErrUnauthenticated, err)
	}
	headerSub := r.Header.Get(HeaderUserSub)
	if headerSub == "" {
		return "", fmt.Errorf("%w: missing %s header", ErrUnauthenticated, HeaderUserSub)
	}
	if token.Subject != headerSub {
		return "", fmt.Errorf("%w: %s does not match token subject", ErrUnauthenticated, HeaderUserSub)
	}
	return token.Subject, nil
}
