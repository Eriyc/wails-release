package gateway

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerServesManifestAndTaggedAssets(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"kid": "test-key",
					"alg": "RS256",
					"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01}),
				},
			},
		})
	}))
	defer jwksServer.Close()

	var githubCalls atomic.Int64
	var assetCalls atomic.Int64
	var githubServer *httptest.Server
	githubServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		githubCalls.Add(1)
		switch r.URL.Path {
		case "/repos/acme/myapp/releases/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         1,
				"tag_name":   "v1.2.3",
				"upload_url": githubServer.URL + "/upload{?name,label}",
				"assets": []map[string]any{
					{
						"id":           10,
						"name":         "manifest.json",
						"url":          githubServer.URL + "/assets/manifest",
						"content_type": "application/json",
					},
				},
			})
		case "/repos/acme/myapp/releases/tags/v1.2.3":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         1,
				"tag_name":   "v1.2.3",
				"upload_url": githubServer.URL + "/upload{?name,label}",
				"assets": []map[string]any{
					{
						"id":           11,
						"name":         "artifact.zip",
						"url":          githubServer.URL + "/assets/artifact",
						"content_type": "application/zip",
					},
				},
			})
		case "/assets/manifest":
			assetCalls.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer gh-secret" {
				t.Fatalf("unexpected manifest asset auth header %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("ETag", `"manifest-etag"`)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/assets/artifact":
			assetCalls.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer gh-secret" {
				t.Fatalf("unexpected artifact auth header %q", got)
			}
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Length", "7")
			w.Header().Set("Last-Modified", time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC).Format(http.TimeFormat))
			_, _ = w.Write([]byte("payload"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer githubServer.Close()

	server, err := NewServer(Config{
		Repository: "acme/myapp",
		Token:      "gh-secret",
		APIBaseURL: githubServer.URL,
		JWKSURL:    jwksServer.URL,
		Issuer:     "https://issuer.example.com",
		Audience:   "wailsrel",
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	validToken := signRS256Token(t, privateKey, "test-key", "https://issuer.example.com", "wailsrel")

	t.Run("manifest ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type %q", got)
		}
		if rec.Header().Get("Authorization") != "" {
			t.Fatalf("expected no authorization response header, got %q", rec.Header().Get("Authorization"))
		}
		body := rec.Body.String()
		if body != `{"ok":true}` {
			t.Fatalf("unexpected body %q", body)
		}
		if strings.Contains(body, "gh-secret") {
			t.Fatalf("response leaked github credential")
		}
	})

	t.Run("tagged download ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/download/v1.2.3/artifact.zip", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/zip" {
			t.Fatalf("unexpected content type %q", got)
		}
		if rec.Header().Get("Location") != "" {
			t.Fatalf("expected proxy response without redirect, got location %q", rec.Header().Get("Location"))
		}
		if rec.Body.String() != "payload" {
			t.Fatalf("unexpected payload %q", rec.Body.String())
		}
	})

	t.Run("invalid jwt rejected", func(t *testing.T) {
		before := githubCalls.Load()
		req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
		req.Header.Set("Authorization", "Bearer invalid.token.value")
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
		if githubCalls.Load() != before {
			t.Fatalf("expected no github calls on invalid jwt")
		}
	})

	if assetCalls.Load() < 2 {
		t.Fatalf("expected asset proxy calls, got %d", assetCalls.Load())
	}
}

func signRS256Token(t *testing.T, key *rsa.PrivateKey, kid, issuer, audience string) string {
	t.Helper()

	header := map[string]any{
		"alg": "RS256",
		"kid": kid,
		"typ": "JWT",
	}
	claims := map[string]any{
		"iss": issuer,
		"aud": audience,
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := encodedHeader + "." + encodedClaims
	sum := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return fmt.Sprintf("%s.%s", signingInput, base64.RawURLEncoding.EncodeToString(signature))
}
