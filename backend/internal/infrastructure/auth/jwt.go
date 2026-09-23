// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package auth provides validation for Heimdall-issued tokens.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/auth0/go-jwt-middleware/v2/jwks"
	"github.com/auth0/go-jwt-middleware/v2/validator"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

const DefaultClockSkew = 5 * time.Second
const ScopeMe = "access:me"
const scopeManageApprovers = "manage:mentorship:approvers"

type contextKey int

const (
	principalKey contextKey = iota
	gatewayPrincipalKey
)

type JWTAuthConfig struct {
	ClockSkew                  time.Duration
	HeimdallJWKSURL            string
	HeimdallAudience           string
	HeimdallIssuer             string
	AllowMockPrincipalBypass   bool
	DisabledMockLocalPrincipal string
}

type HeimdallClaims struct {
	Principal string `json:"principal"`
	Email     string `json:"email,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

func (c *HeimdallClaims) Validate(_ context.Context) error {
	if strings.TrimSpace(c.Principal) == "" {
		return errors.New("principal claim is required")
	}
	return nil
}

var (
	errMissingAuthorizationHeader   = errors.New("missing Authorization header")
	errMalformedAuthorizationHeader = errors.New("malformed Authorization header")
	errMissingBearerToken           = errors.New("missing bearer token")
	errAuthenticatorContextClosed   = errors.New("JWT authenticator context closed")
)

type JWTAuthenticator struct {
	cfg      JWTAuthConfig
	baseCtx  context.Context
	heimdall *validator.Validator
	logger   *slog.Logger
}

func NewJWTAuthenticator(ctx context.Context, cfg JWTAuthConfig, logger *slog.Logger) (*JWTAuthenticator, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	mockPrincipal := strings.TrimSpace(cfg.DisabledMockLocalPrincipal)
	cfg.DisabledMockLocalPrincipal = mockPrincipal
	if mockPrincipal != "" {
		if !cfg.AllowMockPrincipalBypass {
			return nil, errors.New("DISABLED_MOCK_LOCAL_PRINCIPAL requires ALLOW_MOCK_LOCAL_PRINCIPAL_BYPASS=true")
		}
		return &JWTAuthenticator{cfg: cfg, baseCtx: ctx, logger: logger}, nil
	}
	if cfg.HeimdallJWKSURL == "" || cfg.HeimdallAudience == "" || cfg.HeimdallIssuer == "" {
		return nil, errors.New("heimdall JWKS URL, audience, and issuer must be configured together")
	}
	clockSkew := cfg.ClockSkew
	if clockSkew == 0 {
		clockSkew = DefaultClockSkew
	}
	heimdallJWKS, err := url.Parse(cfg.HeimdallJWKSURL)
	if err != nil || !heimdallJWKS.IsAbs() {
		return nil, fmt.Errorf("HEIMDALL_JWKS_URL must be an absolute URL: %w", err)
	}
	issuerURL, err := url.Parse("https://heimdall.invalid")
	if err != nil {
		return nil, fmt.Errorf("build Heimdall issuer URL: %w", err)
	}
	provider := jwks.NewCachingProvider(issuerURL, 5*time.Minute, jwks.WithCustomJWKSURI(heimdallJWKS))
	keyFunc := func(reqCtx context.Context) (interface{}, error) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("%w: %s", errAuthenticatorContextClosed, err)
		}
		return provider.KeyFunc(reqCtx)
	}
	heimdallValidator, err := validator.New(
		keyFunc,
		validator.PS256,
		cfg.HeimdallIssuer,
		[]string{cfg.HeimdallAudience},
		validator.WithCustomClaims(func() validator.CustomClaims { return &HeimdallClaims{} }),
		validator.WithAllowedClockSkew(clockSkew),
	)
	if err != nil {
		return nil, fmt.Errorf("build Heimdall JWT validator: %w", err)
	}
	return &JWTAuthenticator{cfg: cfg, baseCtx: ctx, heimdall: heimdallValidator, logger: logger}, nil
}

func (a *JWTAuthenticator) GatewayMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var principal *models.Principal
		if a.cfg.DisabledMockLocalPrincipal != "" {
			principal = &models.Principal{
				UserID:   a.cfg.DisabledMockLocalPrincipal,
				Username: a.cfg.DisabledMockLocalPrincipal,
				Scope:    ScopeMe,
			}
		} else if r.Header.Get("Authorization") == "" {
			principal = &models.Principal{UserID: "_anonymous", Username: "_anonymous"}
		} else {
			var err error
			principal, err = a.extractHeimdallPrincipal(r)
			if err != nil {
				a.logger.WarnContext(r.Context(), "gateway auth: Heimdall token validation failed", "error", err, "path", r.URL.Path)
				jsonError(w, http.StatusUnauthorized, "invalid or missing gateway token")
				return
			}
		}
		ctx := context.WithValue(ContextWithPrincipal(r.Context(), principal), gatewayPrincipalKey, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *JWTAuthenticator) extractHeimdallPrincipal(r *http.Request) (*models.Principal, error) {
	if a.heimdall == nil {
		if a.cfg.DisabledMockLocalPrincipal != "" {
			return &models.Principal{UserID: a.cfg.DisabledMockLocalPrincipal, Username: a.cfg.DisabledMockLocalPrincipal, Scope: ScopeMe}, nil
		}
		return nil, errors.New("heimdall validator is not configured")
	}
	rawToken, err := bearerToken(r)
	if err != nil {
		return nil, err
	}
	validated, err := a.heimdall.ValidateToken(r.Context(), rawToken)
	if err != nil {
		return nil, fmt.Errorf("validate Heimdall token: %w", err)
	}
	vc, ok := validated.(*validator.ValidatedClaims)
	if !ok {
		return nil, errors.New("unexpected Heimdall claims type")
	}
	claims, ok := vc.CustomClaims.(*HeimdallClaims)
	if !ok {
		return nil, errors.New("unexpected Heimdall custom claims type")
	}
	return &models.Principal{UserID: claims.Principal, Username: claims.Principal, Email: claims.Email, Scope: claims.Scope}, nil
}

func bearerToken(r *http.Request) (string, error) {
	authorization := r.Header.Get("Authorization")
	if authorization == "" {
		return "", errMissingAuthorizationHeader
	}
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errMalformedAuthorizationHeader
	}
	rawToken := strings.TrimSpace(parts[1])
	if rawToken == "" {
		return "", errMissingBearerToken
	}
	return rawToken, nil
}

func IsGatewayPrincipal(ctx context.Context) bool {
	value, _ := ctx.Value(gatewayPrincipalKey).(bool)
	return value
}

func ScopeManageApprovers() string { return scopeManageApprovers }
func ScopeReadMetrics() string     { return "read:mentorship:metrics" }

func HasScope(ctx context.Context, scope string) bool {
	p := PrincipalFromContext(ctx)
	if p == nil {
		return false
	}
	for _, value := range strings.Fields(p.Scope) {
		if value == scope {
			return true
		}
	}
	return false
}

func ContextWithPrincipal(ctx context.Context, p *models.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func PrincipalFromContext(ctx context.Context) *models.Principal {
	p, _ := ctx.Value(principalKey).(*models.Principal)
	return p
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
