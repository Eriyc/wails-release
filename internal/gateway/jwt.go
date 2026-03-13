package gateway

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type JWTVerifier struct {
	jwksURL    string
	issuer     string
	audience   string
	httpClient *http.Client

	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	expires time.Time
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Issuer    string          `json:"iss"`
	Audience  json.RawMessage `json:"aud"`
	Expiry    int64           `json:"exp"`
	NotBefore int64           `json:"nbf"`
}

func NewJWTVerifier(jwksURL, issuer, audience string, httpClient *http.Client) *JWTVerifier {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &JWTVerifier{
		jwksURL:    jwksURL,
		issuer:     issuer,
		audience:   audience,
		httpClient: httpClient,
		keys:       map[string]*rsa.PublicKey{},
	}
}

func (v *JWTVerifier) VerifyRequest(r *http.Request) error {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		return fmt.Errorf("missing bearer token")
	}
	return v.VerifyToken(strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")))
}

func (v *JWTVerifier) VerifyToken(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid token format")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("decode token header: %w", err)
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return fmt.Errorf("parse token header: %w", err)
	}
	if header.Alg != "RS256" {
		return fmt.Errorf("unsupported jwt alg %q", header.Alg)
	}

	key, err := v.lookupKey(context.Background(), header.Kid)
	if err != nil {
		return err
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("decode token signature: %w", err)
	}
	hash := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); err != nil {
		return fmt.Errorf("verify token signature: %w", err)
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("decode token claims: %w", err)
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return fmt.Errorf("parse token claims: %w", err)
	}
	return v.validateClaims(claims)
}

func (v *JWTVerifier) validateClaims(claims jwtClaims) error {
	now := time.Now().Unix()
	if v.issuer != "" && claims.Issuer != v.issuer {
		return fmt.Errorf("unexpected issuer")
	}
	if claims.NotBefore != 0 && now < claims.NotBefore {
		return fmt.Errorf("token not yet valid")
	}
	if claims.Expiry != 0 && now >= claims.Expiry {
		return fmt.Errorf("token expired")
	}

	var audiences []string
	if len(claims.Audience) > 0 {
		if claims.Audience[0] == '"' {
			var single string
			if err := json.Unmarshal(claims.Audience, &single); err != nil {
				return fmt.Errorf("parse token audience: %w", err)
			}
			audiences = append(audiences, single)
		} else {
			if err := json.Unmarshal(claims.Audience, &audiences); err != nil {
				return fmt.Errorf("parse token audience: %w", err)
			}
		}
	}
	for _, candidate := range audiences {
		if candidate == v.audience {
			return nil
		}
	}
	return fmt.Errorf("unexpected audience")
}

func (v *JWTVerifier) lookupKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	expires := v.expires
	v.mu.RUnlock()
	if ok && time.Now().Before(expires) {
		return key, nil
	}

	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok = v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("jwt key %q not found", kid)
	}
	return key, nil
}

func (v *JWTVerifier) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("jwks fetch %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var document jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		return err
	}

	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, jwk := range document.Keys {
		if jwk.Kty != "RSA" || jwk.Kid == "" || jwk.N == "" || jwk.E == "" {
			continue
		}
		key, err := rsaKey(jwk.N, jwk.E)
		if err != nil {
			return err
		}
		keys[jwk.Kid] = key
	}

	v.mu.Lock()
	v.keys = keys
	v.expires = time.Now().Add(5 * time.Minute)
	v.mu.Unlock()
	return nil
}

func rsaKey(modulus, exponent string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e == 0 {
		return nil, fmt.Errorf("invalid rsa exponent")
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}, nil
}
