// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestAuth0MiddlewareRejectsHeimdallTokenOnInterimPath(t *testing.T) {
	authenticator := &JWTAuthenticator{
		cfg: JWTAuthConfig{
			Issuer:   "https://auth.example/",
			Audience: "mentorship-api",
		},
		logger: slog.Default(),
	}
	received := false
	handler := authenticator.Auth0Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		received = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+heimdallShapedToken())
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d; want 401", response.Code)
	}
	if received {
		t.Fatal("interim Auth0 middleware accepted a Heimdall token")
	}
}

func TestOptionalMiddlewareDoesNotResolveHeimdallToken(t *testing.T) {
	authenticator := &JWTAuthenticator{
		cfg: JWTAuthConfig{
			Issuer:   "https://auth.example/",
			Audience: "mentorship-api",
		},
		logger: slog.Default(),
	}
	var gotPrincipal bool
	handler := authenticator.OptionalMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		gotPrincipal = PrincipalFromContext(req.Context()) != nil
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/programs/p1", nil)
	req.Header.Set("Authorization", "Bearer "+heimdallShapedToken())
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotPrincipal {
		t.Fatal("optional interim middleware resolved a Heimdall token")
	}
}

func TestAuth0MiddlewarePreservesValidatedGatewayPrincipal(t *testing.T) {
	authenticator := &JWTAuthenticator{logger: slog.Default()}
	called := false
	handler := authenticator.Auth0Middleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		called = PrincipalFromContext(req.Context()) != nil && IsGatewayPrincipal(req.Context())
	}))
	ctx := context.WithValue(context.Background(), gatewayPrincipalKey, true)
	ctx = ContextWithPrincipal(ctx, &models.Principal{UserID: "local-user"})
	req := httptest.NewRequest(http.MethodGet, "/mentorship/v1/me", nil).WithContext(ctx)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("validated gateway principal was not preserved")
	}
}

func TestNewJWTAuthenticator_RequiresCompleteHeimdallConfig(t *testing.T) {
	_, err := NewJWTAuthenticator(context.Background(), JWTAuthConfig{
		JWKSURL:          "https://auth.example/.well-known/jwks.json",
		Audience:         "mentorship-api",
		Issuer:           "https://auth.example/",
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

func heimdallShapedToken() string {
	encode := func(value string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(value))
	}
	return encode(`{"alg":"none"}`) + "." + encode(`{"iss":"heimdall","principal":"alice"}`) + "."
}
