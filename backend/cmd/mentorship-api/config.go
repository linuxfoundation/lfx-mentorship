// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package main is the entrypoint for the LFX Mentorship API.
package main

import (
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

// Config holds all runtime configuration for the service.
type Config struct {
	Server       ServerConfig
	Database     DatabaseConfig
	JWT          JWTConfig
	FGA          FGAConfig
	Indexer      IndexerConfig
	Email        EmailConfig
	Crowdfunding CrowdfundingConfig
	OTel         OTelConfig
	Local        LocalConfig
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// DatabaseConfig holds PostgreSQL pool settings. Connection details come from
// the environment via db.ConnConfigFromEnv, not from a DSN assembled here.
type DatabaseConfig struct {
	MaxConns        int
	MinConns        int
	ConnMaxLifetime time.Duration
}

// JWTConfig holds Heimdall JWKS settings.
type JWTConfig struct {
	ClockSkew        time.Duration
	HeimdallJWKSURL  string
	HeimdallAudience string
	HeimdallIssuer   string
}

// FGAConfig configures the optional transactional outbox relay.
type FGAConfig struct {
	NATSURL          string
	RelayBatch       int
	RelayInterval    time.Duration
	RelayRetryDelay  time.Duration
	RelayMaxAttempts int
}

// IndexerConfig configures authenticated index publishing and retry behavior.
type IndexerConfig struct {
	// ServiceToken is stamped on every index message; without it the relay idles.
	ServiceToken string
	RetryDelay   time.Duration
	MaxAttempts  int
}

// EmailConfig configures notification email, which is sent over the FGA NATS connection whenever it is set.
type EmailConfig struct {
	// PublicSiteURL is the public Mentorship site that user-facing links point at.
	PublicSiteURL string
	// SelfServeURL is LFX Self Serve, where program management pages live.
	SelfServeURL string
	// HRInbox is the LF staff HR inbox copied on every mentee acceptance.
	HRInbox string
}

// CrowdfundingConfig holds outbound crowdfunding API settings.
type CrowdfundingConfig struct {
	BaseURL string
	Timeout time.Duration
}

// IsConfigured reports whether the crowdfunding client has a base URL.
func (c CrowdfundingConfig) IsConfigured() bool {
	return c.BaseURL != ""
}

// OTelConfig holds OpenTelemetry settings.
type OTelConfig struct {
	ServiceName    string
	ServiceVersion string
	Endpoint       string
}

// LocalConfig holds development-only settings.
type LocalConfig struct {
	// AllowMockLocalPrincipalBypass must be true to enable bypass mode.
	AllowMockLocalPrincipalBypass bool
	// DisabledMockLocalPrincipal sets a static principal for local dev.
	// Leave empty in all non-local environments.
	DisabledMockLocalPrincipal string
	// InviteSecret is the HMAC secret used to sign mentor invite tokens.
	InviteSecret string
}

func loadConfig() (*Config, error) {
	heimdallJWKSURL := os.Getenv("HEIMDALL_JWKS_URL")
	heimdallAudience := os.Getenv("HEIMDALL_JWT_AUDIENCE")
	heimdallIssuer := os.Getenv("HEIMDALL_JWT_ISSUER")
	heimdallConfigured := heimdallJWKSURL != "" || heimdallAudience != "" || heimdallIssuer != ""
	if heimdallConfigured && (heimdallJWKSURL == "" || heimdallAudience == "" || heimdallIssuer == "") {
		return nil, fmt.Errorf("HEIMDALL_JWKS_URL, HEIMDALL_JWT_AUDIENCE, and HEIMDALL_JWT_ISSUER must be set together")
	}
	if !heimdallConfigured && os.Getenv("DISABLED_MOCK_LOCAL_PRINCIPAL") == "" {
		return nil, fmt.Errorf("HEIMDALL_JWKS_URL, HEIMDALL_JWT_AUDIENCE, and HEIMDALL_JWT_ISSUER are required")
	}
	serverPort, err := parseInt(getEnv("PORT", "8080"))
	if err != nil {
		return nil, fmt.Errorf("PORT: %w", err)
	}
	maxConns, err := parseInt(getEnv("DB_MAX_CONNS", "10"))
	if err != nil {
		return nil, fmt.Errorf("DB_MAX_CONNS: %w", err)
	}
	minConns, err := parseInt(getEnv("DB_MIN_CONNS", "2"))
	if err != nil {
		return nil, fmt.Errorf("DB_MIN_CONNS: %w", err)
	}

	clockSkew := 5 * time.Second
	if v := os.Getenv("JWT_CLOCK_SKEW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("JWT_CLOCK_SKEW: %w", err)
		}
		clockSkew = d
	}

	relayBatch, err := parseInt(getEnv("FGA_RELAY_BATCH_SIZE", "50"))
	if err != nil || relayBatch <= 0 {
		return nil, fmt.Errorf("FGA_RELAY_BATCH_SIZE: must be a positive integer")
	}
	relayMaxAttempts, err := parseInt(getEnv("FGA_RELAY_MAX_ATTEMPTS", "10"))
	if err != nil || relayMaxAttempts <= 0 {
		return nil, fmt.Errorf("FGA_RELAY_MAX_ATTEMPTS: must be a positive integer")
	}
	relayInterval := time.Second
	if v := os.Getenv("FGA_RELAY_INTERVAL"); v != "" {
		relayInterval, err = time.ParseDuration(v)
		if err != nil || relayInterval <= 0 {
			return nil, fmt.Errorf("FGA_RELAY_INTERVAL: must be a positive duration")
		}
	}
	relayRetryDelay := time.Minute
	if v := os.Getenv("FGA_RELAY_RETRY_DELAY"); v != "" {
		relayRetryDelay, err = time.ParseDuration(v)
		if err != nil || relayRetryDelay <= 0 {
			return nil, fmt.Errorf("FGA_RELAY_RETRY_DELAY: must be a positive duration")
		}
	}
	indexRetryDelay := time.Minute
	if v := os.Getenv("INDEX_RELAY_RETRY_DELAY"); v != "" {
		indexRetryDelay, err = time.ParseDuration(v)
		if err != nil || indexRetryDelay <= 0 {
			return nil, fmt.Errorf("INDEX_RELAY_RETRY_DELAY: must be a positive duration")
		}
	}
	indexMaxAttempts, err := parseInt(getEnv("INDEX_RELAY_MAX_ATTEMPTS", "10"))
	if err != nil || indexMaxAttempts <= 0 {
		return nil, fmt.Errorf("INDEX_RELAY_MAX_ATTEMPTS: must be a positive integer")
	}
	crowdfundingTimeout := 10 * time.Second
	if v := os.Getenv("CROWDFUNDING_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("CROWDFUNDING_TIMEOUT: %w", err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("CROWDFUNDING_TIMEOUT: must be greater than 0")
		}
		crowdfundingTimeout = d
	}
	emailCfg, err := loadEmailConfig(os.Getenv("FGA_NATS_URL") != "")
	if err != nil {
		return nil, err
	}

	return &Config{
		Server: ServerConfig{
			Port:            serverPort,
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Database: DatabaseConfig{
			MaxConns:        maxConns,
			MinConns:        minConns,
			ConnMaxLifetime: 30 * time.Minute,
		},
		JWT: JWTConfig{
			ClockSkew:        clockSkew,
			HeimdallJWKSURL:  heimdallJWKSURL,
			HeimdallAudience: heimdallAudience,
			HeimdallIssuer:   heimdallIssuer,
		},
		FGA: FGAConfig{
			NATSURL:          os.Getenv("FGA_NATS_URL"),
			RelayBatch:       relayBatch,
			RelayInterval:    relayInterval,
			RelayRetryDelay:  relayRetryDelay,
			RelayMaxAttempts: relayMaxAttempts,
		},
		Indexer: IndexerConfig{
			ServiceToken: os.Getenv("INDEXER_SERVICE_TOKEN"),
			RetryDelay:   indexRetryDelay,
			MaxAttempts:  indexMaxAttempts,
		},
		Email: emailCfg,
		Crowdfunding: CrowdfundingConfig{
			BaseURL: strings.TrimRight(os.Getenv("CROWDFUNDING_BASE_URL"), "/"),
			Timeout: crowdfundingTimeout,
		},
		OTel: OTelConfig{
			ServiceName:    getEnv("OTEL_SERVICE_NAME", "lfx-mentorship-api"),
			ServiceVersion: getEnv("OTEL_SERVICE_VERSION", "dev"),
			Endpoint:       os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		},
		Local: LocalConfig{
			AllowMockLocalPrincipalBypass: os.Getenv("ALLOW_MOCK_LOCAL_PRINCIPAL_BYPASS") == "true",
			DisabledMockLocalPrincipal:    os.Getenv("DISABLED_MOCK_LOCAL_PRINCIPAL"),
			InviteSecret:                  requireEnv("MENTOR_INVITE_SECRET"),
		},
	}, nil
}

// loadEmailConfig reads notification email settings, which are required once NATS is configured.
func loadEmailConfig(natsConfigured bool) (EmailConfig, error) {
	cfg := EmailConfig{
		PublicSiteURL: strings.TrimRight(os.Getenv("PUBLIC_SITE_URL"), "/"),
		SelfServeURL:  strings.TrimRight(os.Getenv("SELF_SERVE_URL"), "/"),
		HRInbox:       strings.TrimSpace(os.Getenv("EMAIL_HR_INBOX")),
	}
	if !natsConfigured {
		return cfg, nil
	}
	for _, kv := range [][2]string{{"PUBLIC_SITE_URL", cfg.PublicSiteURL}, {"SELF_SERVE_URL", cfg.SelfServeURL}} {
		if u, err := url.Parse(kv[1]); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return EmailConfig{}, fmt.Errorf("%s must be an absolute http(s) URL with no query or fragment when FGA_NATS_URL is set", kv[0])
		}
	}
	if addr, err := mail.ParseAddress(cfg.HRInbox); err != nil || addr.Address != cfg.HRInbox {
		return EmailConfig{}, fmt.Errorf("EMAIL_HR_INBOX must be a bare email address when FGA_NATS_URL is set")
	}
	return cfg, nil
}

// jwtAuthConfig converts JWTConfig into an auth.JWTAuthConfig.
func (c *Config) jwtAuthConfig() auth.JWTAuthConfig {
	return auth.JWTAuthConfig{
		ClockSkew:                  c.JWT.ClockSkew,
		HeimdallJWKSURL:            c.JWT.HeimdallJWKSURL,
		HeimdallAudience:           c.JWT.HeimdallAudience,
		HeimdallIssuer:             c.JWT.HeimdallIssuer,
		AllowMockPrincipalBypass:   c.Local.AllowMockLocalPrincipalBypass,
		DisabledMockLocalPrincipal: c.Local.DisabledMockLocalPrincipal,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "fatal: required environment variable %q is not set\n", key)
		os.Exit(1)
	}
	return v
}

func parseInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("must be an integer, got %q", s)
	}
	return n, nil
}
