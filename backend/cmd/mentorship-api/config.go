// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package main is the entrypoint for the LFX Mentorship API.
package main

import (
	"fmt"
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

// JWTConfig holds Auth0 / JWKS settings.
type JWTConfig struct {
	JWKSURL          string
	Audience         string
	Issuer           string
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

// CrowdfundingConfig holds outbound crowdfunding API and M2M auth settings.
type CrowdfundingConfig struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	Audience     string
	Scope        string
	Timeout      time.Duration
}

// IsConfigured reports whether all required crowdfunding client settings are present.
func (c CrowdfundingConfig) IsConfigured() bool {
	return c.BaseURL != "" && c.TokenURL != "" && c.ClientID != "" && c.ClientSecret != "" && c.Audience != ""
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
	if heimdallConfigured && heimdallIssuer == os.Getenv("JWT_ISSUER") {
		return nil, fmt.Errorf("HEIMDALL_JWT_ISSUER must differ from JWT_ISSUER")
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
			JWKSURL:          os.Getenv("JWKS_URL"),
			Audience:         os.Getenv("JWT_AUDIENCE"),
			Issuer:           os.Getenv("JWT_ISSUER"),
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
		Crowdfunding: CrowdfundingConfig{
			BaseURL:      strings.TrimRight(os.Getenv("CROWDFUNDING_BASE_URL"), "/"),
			TokenURL:     os.Getenv("CROWDFUNDING_TOKEN_URL"),
			ClientID:     os.Getenv("CROWDFUNDING_CLIENT_ID"),
			ClientSecret: os.Getenv("CROWDFUNDING_CLIENT_SECRET"),
			Audience:     os.Getenv("CROWDFUNDING_AUDIENCE"),
			Scope:        getEnv("CROWDFUNDING_SCOPE", "access:manage"),
			Timeout:      crowdfundingTimeout,
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

// jwtAuthConfig converts JWTConfig into an auth.JWTAuthConfig.
func (c *Config) jwtAuthConfig() auth.JWTAuthConfig {
	return auth.JWTAuthConfig{
		JWKSURL:                    c.JWT.JWKSURL,
		Audience:                   c.JWT.Audience,
		Issuer:                     c.JWT.Issuer,
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
