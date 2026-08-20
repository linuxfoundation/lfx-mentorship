// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package auth provides JWT validation for Auth0-issued tokens.
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

// DefaultClockSkew is the default leeway applied when validating JWT expiry.
const DefaultClockSkew = 5 * time.Second

// ScopeMe is the OAuth2 scope required for authenticated user routes.
const ScopeMe = "access:me"

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey int

const principalKey contextKey = iota

// JWTAuthConfig configures the JWT authenticator.
type JWTAuthConfig struct {
	JWKSURL   string
	Audience  string
	Issuer    string
	ClockSkew time.Duration
	// AllowMockPrincipalBypass must be true to permit DisabledMockLocalPrincipal.
	AllowMockPrincipalBypass bool
	// DisabledMockLocalPrincipal sets a static principal for local dev — empty in production.
	DisabledMockLocalPrincipal string
}

// JWTClaims extends standard JWT claims with LFX-specific fields.
type JWTClaims struct {
	Subject       string `json:"sub"`
	Username      string `json:"https://sso.linuxfoundation.org/claims/username"`
	SSOEmail      string `json:"https://sso.linuxfoundation.org/claims/email"`
	Scope         string `json:"scope"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

func (c *JWTClaims) effectiveEmail() string {
	if v := strings.TrimSpace(c.SSOEmail); v != "" {
		return v
	}
	return strings.TrimSpace(c.Email)
}

// Validate satisfies validator.CustomClaims.
func (c *JWTClaims) Validate(_ context.Context) error { return nil }

var (
	errMissingAuthorizationHeader   = errors.New("missing Authorization header")
	errMalformedAuthorizationHeader = errors.New("malformed Authorization header")
	errMissingBearerToken           = errors.New("missing bearer token")
	errAuthenticatorContextClosed   = errors.New("JWT authenticator context closed")
)

// JWTAuthenticator validates JWTs using a JWKS endpoint.
type JWTAuthenticator struct {
	cfg       JWTAuthConfig
	baseCtx   context.Context
	validator *validator.Validator
	logger    *slog.Logger
}

// NewJWTAuthenticator creates a JWTAuthenticator backed by the given JWKS URL.
func NewJWTAuthenticator(ctx context.Context, cfg JWTAuthConfig, logger *slog.Logger) (*JWTAuthenticator, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	mockPrincipal := strings.TrimSpace(cfg.DisabledMockLocalPrincipal)
	cfg.DisabledMockLocalPrincipal = mockPrincipal // normalise before storage to prevent whitespace bypass
	if mockPrincipal != "" {
		if !cfg.AllowMockPrincipalBypass {
			return nil, errors.New("DISABLED_MOCK_LOCAL_PRINCIPAL requires ALLOW_MOCK_LOCAL_PRINCIPAL_BYPASS=true")
		}
		return &JWTAuthenticator{cfg: cfg, baseCtx: ctx, logger: logger}, nil
	}

	clockSkew := cfg.ClockSkew
	if clockSkew == 0 {
		clockSkew = DefaultClockSkew
	}

	issuerURL, err := url.Parse(cfg.Issuer)
	if err != nil || !issuerURL.IsAbs() {
		return nil, fmt.Errorf("JWT_ISSUER must be an absolute URL: %w", err)
	}
	jwksURL, err := url.Parse(cfg.JWKSURL)
	if err != nil || !jwksURL.IsAbs() {
		return nil, fmt.Errorf("JWKS_URL must be an absolute URL: %w", err)
	}

	provider := jwks.NewCachingProvider(issuerURL, 5*time.Minute, jwks.WithCustomJWKSURI(jwksURL))
	keyFunc := func(reqCtx context.Context) (interface{}, error) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("%w: %s", errAuthenticatorContextClosed, err)
		}
		return provider.KeyFunc(reqCtx)
	}

	jwtValidator, err := validator.New(
		keyFunc,
		validator.RS256,
		cfg.Issuer,
		[]string{cfg.Audience},
		validator.WithCustomClaims(func() validator.CustomClaims { return &JWTClaims{} }),
		validator.WithAllowedClockSkew(clockSkew),
	)
	if err != nil {
		return nil, fmt.Errorf("build JWT validator: %w", err)
	}

	return &JWTAuthenticator{cfg: cfg, baseCtx: ctx, validator: jwtValidator, logger: logger}, nil
}

// Middleware returns an http.Handler middleware that validates the Bearer token.
// Returns 401 on failure.
func (a *JWTAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.DisabledMockLocalPrincipal != "" {
			p := &models.Principal{
				UserID:        a.cfg.DisabledMockLocalPrincipal,
				Username:      a.cfg.DisabledMockLocalPrincipal,
				Email:         a.cfg.DisabledMockLocalPrincipal + "@local.dev",
				EmailVerified: true,
				Scope:         ScopeMe,
			}
			next.ServeHTTP(w, r.WithContext(ContextWithPrincipal(r.Context(), p)))
			return
		}

		claims, err := a.extractAndValidate(r)
		if err != nil {
			a.logger.WarnContext(r.Context(), "auth: token validation failed", "error", err, "path", r.URL.Path)
			jsonError(w, http.StatusUnauthorized, "invalid or missing token")
			return
		}

		if strings.TrimSpace(claims.Subject) == "" {
			jsonError(w, http.StatusUnauthorized, "invalid token claims")
			return
		}

		principal := &models.Principal{
			UserID:        claims.Subject,
			Username:      claims.Username,
			Scope:         claims.Scope,
			Email:         claims.effectiveEmail(),
			EmailVerified: claims.EmailVerified,
			Name:          claims.Name,
			GivenName:     claims.GivenName,
			FamilyName:    claims.FamilyName,
			Picture:       claims.Picture,
		}
		next.ServeHTTP(w, r.WithContext(ContextWithPrincipal(r.Context(), principal)))
	})
}

// OptionalMiddleware is like Middleware but never rejects the request.
func (a *JWTAuthenticator) OptionalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.DisabledMockLocalPrincipal != "" {
			p := &models.Principal{
				UserID:   a.cfg.DisabledMockLocalPrincipal,
				Username: a.cfg.DisabledMockLocalPrincipal,
				Scope:    ScopeMe,
			}
			next.ServeHTTP(w, r.WithContext(ContextWithPrincipal(r.Context(), p)))
			return
		}

		if claims, err := a.extractAndValidate(r); err == nil && claims != nil && claims.Subject != "" {
			p := &models.Principal{
				UserID:        claims.Subject,
				Username:      claims.Username,
				Scope:         claims.Scope,
				Email:         claims.effectiveEmail(),
				EmailVerified: claims.EmailVerified,
				Name:          claims.Name,
				GivenName:     claims.GivenName,
				FamilyName:    claims.FamilyName,
				Picture:       claims.Picture,
			}
			r = r.WithContext(ContextWithPrincipal(r.Context(), p))
		}
		next.ServeHTTP(w, r)
	})
}

func (a *JWTAuthenticator) extractAndValidate(r *http.Request) (*JWTClaims, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errMissingAuthorizationHeader
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, errMalformedAuthorizationHeader
	}
	rawToken := strings.TrimSpace(parts[1])
	if rawToken == "" {
		return nil, errMissingBearerToken
	}

	if a.validator == nil {
		return nil, errors.New("JWT validator is not configured")
	}

	validatedClaims, err := a.validator.ValidateToken(r.Context(), rawToken)
	if err != nil {
		return nil, fmt.Errorf("validate token: %w", err)
	}

	vc, ok := validatedClaims.(*validator.ValidatedClaims)
	if !ok {
		return nil, errors.New("unexpected claims type")
	}
	custom, ok := vc.CustomClaims.(*JWTClaims)
	if !ok {
		return nil, errors.New("unexpected custom claims type")
	}
	custom.Subject = vc.RegisteredClaims.Subject
	return custom, nil
}

// HasScope reports whether the principal in ctx has the given OAuth2 scope.
func HasScope(ctx context.Context, scope string) bool {
	p := PrincipalFromContext(ctx)
	if p == nil {
		return false
	}
	for _, s := range strings.Fields(p.Scope) {
		if s == scope {
			return true
		}
	}
	return false
}

// ContextWithPrincipal stores the principal in ctx.
func ContextWithPrincipal(ctx context.Context, p *models.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext retrieves the principal from ctx, or nil if absent.
func PrincipalFromContext(ctx context.Context) *models.Principal {
	p, _ := ctx.Value(principalKey).(*models.Principal)
	return p
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
