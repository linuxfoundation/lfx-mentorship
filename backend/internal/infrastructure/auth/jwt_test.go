// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gopkg.in/go-jose/go-jose.v2"
)

func TestGatewayMiddlewareMarksAnonymousRequests(t *testing.T) {
	authenticator := &JWTAuthenticator{logger: slog.Default()}
	called := false
	handler := authenticator.GatewayMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		called = PrincipalFromContext(req.Context()).UserID == "_anonymous" && IsGatewayPrincipal(req.Context())
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/mentorship/v1/programs", nil))
	if !called {
		t.Fatal("gateway middleware did not mark anonymous request")
	}
}

func TestGatewayMiddlewareUsesConfiguredMockPrincipal(t *testing.T) {
	authenticator := &JWTAuthenticator{
		cfg:    JWTAuthConfig{DisabledMockLocalPrincipal: "local-user"},
		logger: slog.Default(),
	}
	handler := authenticator.GatewayMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		principal := PrincipalFromContext(req.Context())
		if principal == nil || principal.UserID != "local-user" || principal.Scope != ScopeMe {
			t.Fatalf("got principal %#v; want local mock principal", principal)
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/mentorship/v1/me", nil))
}

func TestNewJWTAuthenticator_RequiresCompleteHeimdallConfig(t *testing.T) {
	_, err := NewJWTAuthenticator(context.Background(), JWTAuthConfig{
		HeimdallJWKSURL:  "http://heimdall.local/jwks",
		HeimdallAudience: "lfx-mentorship-backend",
	}, slog.Default())
	if err == nil {
		t.Fatal("expected incomplete Heimdall configuration to be rejected")
	}
}

func TestHeimdallClaimsRequirePrincipal(t *testing.T) {
	if err := (&HeimdallClaims{}).Validate(context.Background()); err == nil {
		t.Fatal("expected missing principal claim to be rejected")
	}
}

func TestGatewayMiddlewareValidatesPS256HeimdallToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key:       &privateKey.PublicKey,
			KeyID:     "test-key",
			Algorithm: string(jose.PS256),
			Use:       "sig",
		}}})
	}))
	defer jwksServer.Close()

	claims := heimdallTokenClaims{
		Issuer:    "heimdall",
		Audience:  "lfx-mentorship-backend",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		Principal: "alice",
	}
	tests := []struct {
		name      string
		claims    heimdallTokenClaims
		algorithm jose.SignatureAlgorithm
		wantCode  int
	}{
		{name: "valid", claims: claims, algorithm: jose.PS256, wantCode: http.StatusOK},
		{name: "wrong issuer", claims: withIssuer(claims, "other"), algorithm: jose.PS256, wantCode: http.StatusUnauthorized},
		{name: "wrong audience", claims: withAudience(claims, "other"), algorithm: jose.PS256, wantCode: http.StatusUnauthorized},
		{name: "expired", claims: withExpiry(claims, time.Now().Add(-time.Hour)), algorithm: jose.PS256, wantCode: http.StatusUnauthorized},
		{name: "wrong algorithm", claims: claims, algorithm: jose.RS256, wantCode: http.StatusUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token := signedHeimdallToken(t, privateKey, test.algorithm, test.claims)
			authenticator, err := NewJWTAuthenticator(context.Background(), JWTAuthConfig{
				HeimdallJWKSURL:  jwksServer.URL,
				HeimdallAudience: "lfx-mentorship-backend",
				HeimdallIssuer:   "heimdall",
			}, slog.Default())
			if err != nil {
				t.Fatalf("new authenticator: %v", err)
			}
			handler := authenticator.GatewayMiddleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				principal := PrincipalFromContext(req.Context())
				if principal == nil || principal.UserID != "alice" {
					t.Fatalf("got principal %#v; want alice", principal)
				}
				w.WriteHeader(http.StatusOK)
			}))
			request := httptest.NewRequest(http.MethodGet, "/mentorship/v1/me", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantCode {
				t.Fatalf("got status %d; want %d", response.Code, test.wantCode)
			}
		})
	}
}

type heimdallTokenClaims struct {
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	ExpiresAt int64  `json:"exp"`
	Principal string `json:"principal"`
}

func signedHeimdallToken(t *testing.T, privateKey *rsa.PrivateKey, algorithm jose.SignatureAlgorithm, claims heimdallTokenClaims) string {
	t.Helper()
	options := (&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), "test-key")
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: algorithm, Key: privateKey}, options)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	serialized, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	token, err := serialized.CompactSerialize()
	if err != nil {
		t.Fatalf("serialize token: %v", err)
	}
	return token
}

func withIssuer(claims heimdallTokenClaims, issuer string) heimdallTokenClaims {
	claims.Issuer = issuer
	return claims
}

func withAudience(claims heimdallTokenClaims, audience string) heimdallTokenClaims {
	claims.Audience = audience
	return claims
}

func withExpiry(claims heimdallTokenClaims, expiry time.Time) heimdallTokenClaims {
	claims.ExpiresAt = expiry.Unix()
	return claims
}
