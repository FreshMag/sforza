package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/francesco/sforza/internal/auth"
)

func TestStaticAuthenticator(t *testing.T) {
	a := auth.Static{DefaultSub: "test-user"}

	req := httptest.NewRequest("GET", "/", nil)
	sub, err := a.Authenticate(req)
	if err != nil || sub != "test-user" {
		t.Errorf("default sub: got (%q, %v), want (test-user, nil)", sub, err)
	}

	req.Header.Set(auth.HeaderUserSub, "pippo")
	sub, err = a.Authenticate(req)
	if err != nil || sub != "pippo" {
		t.Errorf("header sub: got (%q, %v), want (pippo, nil)", sub, err)
	}

	empty := auth.Static{}
	if _, err := empty.Authenticate(httptest.NewRequest("GET", "/", nil)); err == nil {
		t.Error("empty authenticator without header must fail")
	}
}

// fakeProvider is a minimal OIDC issuer: discovery document + JWKS.
type fakeProvider struct {
	srv *httptest.Server
	key *rsa.PrivateKey
}

func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &fakeProvider{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                p.srv.URL,
			"jwks_uri":                              p.srv.URL + "/keys",
			"authorization_endpoint":                p.srv.URL + "/auth",
			"token_endpoint":                        p.srv.URL + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "test-key",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakeProvider) token(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	if _, ok := claims["iss"]; !ok {
		claims["iss"] = p.srv.URL
	}
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}
	claims["iat"] = time.Now().Add(-time.Minute).Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(p.key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func request(token, headerSub string) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if headerSub != "" {
		req.Header.Set(auth.HeaderUserSub, headerSub)
	}
	return req
}

func TestOIDCAuthenticator(t *testing.T) {
	p := newFakeProvider(t)
	a, err := auth.NewOIDC(context.Background(), p.srv.URL, "sforza")
	if err != nil {
		t.Fatalf("NewOIDC: %v", err)
	}

	valid := p.token(t, jwt.MapClaims{"sub": "pippo", "aud": "sforza"})

	t.Run("valid token with matching header", func(t *testing.T) {
		sub, err := a.Authenticate(request(valid, "pippo"))
		if err != nil || sub != "pippo" {
			t.Fatalf("got (%q, %v), want (pippo, nil)", sub, err)
		}
	})

	t.Run("missing bearer token", func(t *testing.T) {
		if _, err := a.Authenticate(request("", "pippo")); err == nil {
			t.Fatal("must reject request without token")
		}
	})

	t.Run("missing X-User-Sub header", func(t *testing.T) {
		if _, err := a.Authenticate(request(valid, "")); err == nil {
			t.Fatal("must reject request without X-User-Sub")
		}
	})

	t.Run("sub mismatch", func(t *testing.T) {
		if _, err := a.Authenticate(request(valid, "mallory")); err == nil {
			t.Fatal("must reject X-User-Sub != JWT.sub")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		expired := p.token(t, jwt.MapClaims{
			"sub": "pippo", "aud": "sforza", "exp": time.Now().Add(-time.Hour).Unix(),
		})
		if _, err := a.Authenticate(request(expired, "pippo")); err == nil {
			t.Fatal("must reject expired token")
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		wrongAud := p.token(t, jwt.MapClaims{"sub": "pippo", "aud": "other-service"})
		if _, err := a.Authenticate(request(wrongAud, "pippo")); err == nil {
			t.Fatal("must reject wrong audience")
		}
	})

	t.Run("forged signature", func(t *testing.T) {
		other := newFakeProvider(t) // different key, same claims
		forged := other.token(t, jwt.MapClaims{
			"iss": p.srv.URL, "sub": "pippo", "aud": "sforza",
		})
		if _, err := a.Authenticate(request(forged, "pippo")); err == nil {
			t.Fatal("must reject token signed by an unknown key")
		}
	})

	t.Run("garbage token", func(t *testing.T) {
		if _, err := a.Authenticate(request("not.a.jwt", "pippo")); err == nil {
			t.Fatal("must reject malformed token")
		}
	})
}

func TestOIDCAudienceSkippedWhenUnset(t *testing.T) {
	p := newFakeProvider(t)
	a, err := auth.NewOIDC(context.Background(), p.srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	token := p.token(t, jwt.MapClaims{"sub": "pippo", "aud": "anything"})
	sub, err := a.Authenticate(request(token, "pippo"))
	if err != nil || sub != "pippo" {
		t.Fatalf("got (%q, %v), want (pippo, nil)", sub, err)
	}
}
