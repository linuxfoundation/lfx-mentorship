// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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
