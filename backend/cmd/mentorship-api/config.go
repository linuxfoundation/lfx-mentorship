// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package main is the entrypoint for the LFX Mentorship API.
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

// Config holds all runtime configuration for the service.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	OTel     OTelConfig
	Local    LocalConfig
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	DSN             string
	MaxConns        int
	MinConns        int
	ConnMaxLifetime time.Duration
}

// JWTConfig holds Auth0 / JWKS settings.
type JWTConfig struct {
	JWKSURL   string
	Audience  string
	Issuer    string
	ClockSkew time.Duration
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

	return &Config{
		Server: ServerConfig{
			Port:            serverPort,
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Database: DatabaseConfig{
			DSN:             requireEnv("DATABASE_DSN"),
			MaxConns:        maxConns,
			MinConns:        minConns,
			ConnMaxLifetime: 30 * time.Minute,
		},
		JWT: JWTConfig{
			JWKSURL:   os.Getenv("JWKS_URL"),
			Audience:  os.Getenv("JWT_AUDIENCE"),
			Issuer:    os.Getenv("JWT_ISSUER"),
			ClockSkew: clockSkew,
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
