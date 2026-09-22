// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package auth provides JWT validation for Auth0-issued tokens.
package auth

import (
	"context"
	"encoding/base64"
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

const (
	scopeManageApprovers     = "manage:mentorship:approvers"
	scopeManageProjectAdmins = "manage:mentorship:project-admins"
)

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey int

const (
	principalKey contextKey = iota
	gatewayPrincipalKey
)

// JWTAuthConfig configures the JWT authenticator.
type JWTAuthConfig struct {
	JWKSURL          string
	Audience         string
	Issuer           string
	ClockSkew        time.Duration
	HeimdallJWKSURL  string
	HeimdallAudience string
	HeimdallIssuer   string
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

// HeimdallClaims contains the identity forwarded by the platform gateway.
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
	heimdall  *validator.Validator
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
	heimdallConfigured := cfg.HeimdallJWKSURL != "" || cfg.HeimdallAudience != "" || cfg.HeimdallIssuer != ""
	if heimdallConfigured && (cfg.HeimdallJWKSURL == "" || cfg.HeimdallAudience == "" || cfg.HeimdallIssuer == "") {
		return nil, errors.New("heimdall JWKS URL, audience, and issuer must be configured together")
	}
	if heimdallConfigured && cfg.HeimdallIssuer == cfg.Issuer {
		return nil, errors.New("heimdall issuer must differ from Auth0 issuer")
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

	authenticator := &JWTAuthenticator{cfg: cfg, baseCtx: ctx, validator: jwtValidator, logger: logger}
	if heimdallConfigured {
		heimdallJWKS, err := url.Parse(cfg.HeimdallJWKSURL)
		if err != nil || !heimdallJWKS.IsAbs() {
			return nil, fmt.Errorf("HEIMDALL_JWKS_URL must be an absolute URL: %w", err)
		}
		heimdallIssuerURL, err := url.Parse("https://heimdall.invalid")
		if err != nil {
			return nil, fmt.Errorf("build Heimdall issuer URL: %w", err)
		}
		provider := jwks.NewCachingProvider(heimdallIssuerURL, 5*time.Minute, jwks.WithCustomJWKSURI(heimdallJWKS))
		heimdallKeyFunc := func(reqCtx context.Context) (interface{}, error) {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("%w: %s", errAuthenticatorContextClosed, err)
			}
			return provider.KeyFunc(reqCtx)
		}
		heimdallValidator, err := validator.New(
			heimdallKeyFunc,
			validator.PS256,
			cfg.HeimdallIssuer,
			[]string{cfg.HeimdallAudience},
			validator.WithCustomClaims(func() validator.CustomClaims { return &HeimdallClaims{} }),
			validator.WithAllowedClockSkew(clockSkew),
		)
		if err != nil {
			return nil, fmt.Errorf("build Heimdall JWT validator: %w", err)
		}
		authenticator.heimdall = heimdallValidator
	}
	return authenticator, nil
}

// Middleware returns the Auth0-only middleware used by the interim host.
func (a *JWTAuthenticator) Middleware(next http.Handler) http.Handler {
	return a.Auth0Middleware(next)
}

// Auth0Middleware validates only the configured Auth0 issuer. Heimdall tokens
// are accepted only after GatewayMiddleware has run on the gateway mount.
func (a *JWTAuthenticator) Auth0Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := PrincipalFromContext(r.Context()); p != nil {
			if p.UserID == "_anonymous" {
				jsonError(w, http.StatusUnauthorized, "authenticated principal is required")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
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

		principal, err := a.extractAuth0Principal(r)
		if err != nil {
			a.logger.WarnContext(r.Context(), "auth: token validation failed", "error", err, "path", r.URL.Path)
			jsonError(w, http.StatusUnauthorized, "invalid or missing token")
			return
		}

		next.ServeHTTP(w, r.WithContext(ContextWithPrincipal(r.Context(), principal)))
	})
}

// GatewayMiddleware accepts anonymous gateway requests and validates Heimdall tokens when present.
func (a *JWTAuthenticator) GatewayMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var principal *models.Principal
		isGateway := false
		if r.Header.Get("Authorization") == "" {
			principal = &models.Principal{UserID: "_anonymous", Username: "_anonymous"}
			isGateway = true
		} else {
			var err error
			principal, err = a.extractHeimdallPrincipal(r)
			if err != nil {
				a.logger.WarnContext(r.Context(), "gateway auth: Heimdall token validation failed", "error", err, "path", r.URL.Path)
				jsonError(w, http.StatusUnauthorized, "invalid or missing gateway token")
				return
			}
			isGateway = true
		}
		ctx := ContextWithPrincipal(r.Context(), principal)
		if isGateway {
			ctx = context.WithValue(ctx, gatewayPrincipalKey, true)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalMiddleware is like Middleware but never rejects the request.
func (a *JWTAuthenticator) OptionalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if PrincipalFromContext(r.Context()) != nil {
			next.ServeHTTP(w, r)
			return
		}
		if a.cfg.DisabledMockLocalPrincipal != "" {
			p := &models.Principal{
				UserID:   a.cfg.DisabledMockLocalPrincipal,
				Username: a.cfg.DisabledMockLocalPrincipal,
				Scope:    ScopeMe,
			}
			next.ServeHTTP(w, r.WithContext(ContextWithPrincipal(r.Context(), p)))
			return
		}

		if principal, err := a.extractAuth0Principal(r); err == nil && principal != nil {
			r = r.WithContext(ContextWithPrincipal(r.Context(), principal))
		}
		next.ServeHTTP(w, r)
	})
}

func (a *JWTAuthenticator) extractAuth0Principal(r *http.Request) (*models.Principal, error) {
	rawToken, err := bearerToken(r)
	if err != nil {
		return nil, err
	}
	issuer, err := unverifiedIssuer(rawToken)
	if err != nil {
		return nil, err
	}
	if issuer != a.cfg.Issuer {
		return nil, errors.New("token issuer is not configured")
	}
	claims, err := a.extractAndValidate(r)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, errors.New("subject claim is required")
	}
	return &models.Principal{
		UserID:        claims.Subject,
		Username:      claims.Username,
		Scope:         claims.Scope,
		Email:         claims.effectiveEmail(),
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
		GivenName:     claims.GivenName,
		FamilyName:    claims.FamilyName,
		Picture:       claims.Picture,
	}, nil
}

func (a *JWTAuthenticator) extractHeimdallPrincipal(r *http.Request) (*models.Principal, error) {
	if a.heimdall == nil {
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
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errMissingAuthorizationHeader
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errMalformedAuthorizationHeader
	}
	rawToken := strings.TrimSpace(parts[1])
	if rawToken == "" {
		return "", errMissingBearerToken
	}
	return rawToken, nil
}

func unverifiedIssuer(rawToken string) (string, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("invalid token payload")
	}
	var claims struct {
		Issuer string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Issuer == "" {
		return "", errors.New("token issuer is missing")
	}
	return claims.Issuer, nil
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

func isGatewayPrincipal(ctx context.Context) bool {
	value, _ := ctx.Value(gatewayPrincipalKey).(bool)
	return value
}

// IsGatewayPrincipal reports whether the request was authenticated by the
// Heimdall platform middleware rather than the interim Auth0 path.
func IsGatewayPrincipal(ctx context.Context) bool { return isGatewayPrincipal(ctx) }

func ScopeManageApprovers() string     { return scopeManageApprovers }
func ScopeManageProjectAdmins() string { return scopeManageProjectAdmins }
func ScopeReadMetrics() string         { return "read:mentorship:metrics" }

// HasProjectManagementScope accepts the global platform scope or a scope
// restricted to the requested project UID.
func HasProjectManagementScope(ctx context.Context, projectUID string) bool {
	return HasScope(ctx, scopeManageProjectAdmins) || HasScope(ctx, scopeManageProjectAdmins+":"+projectUID)
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
