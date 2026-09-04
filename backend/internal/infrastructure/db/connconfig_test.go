// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"strings"
	"testing"
)

// clearDBEnv unsets every variable ConnConfigFromEnv reads, so each test starts
// from a known state regardless of the developer's own environment.
func clearDBEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE", "DATABASE_DSN"} {
		t.Setenv(k, "")
	}
}

func TestConnConfigFromEnvDiscrete(t *testing.T) {
	clearDBEnv(t)
	t.Setenv("DB_HOST", "rds.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "mentorship")
	t.Setenv("DB_PASSWORD", "s3cret")
	t.Setenv("DB_NAME", "mentorship")

	cfg, err := ConnConfigFromEnv()
	if err != nil {
		t.Fatalf("ConnConfigFromEnv: %v", err)
	}
	if cfg.Host != "rds.example.com" {
		t.Errorf("Host = %q, want rds.example.com", cfg.Host)
	}
	if cfg.Port != 5433 {
		t.Errorf("Port = %d, want 5433", cfg.Port)
	}
	if cfg.User != "mentorship" {
		t.Errorf("User = %q, want mentorship", cfg.User)
	}
	if cfg.Password != "s3cret" {
		t.Errorf("Password = %q, want s3cret", cfg.Password)
	}
	if cfg.Database != "mentorship" {
		t.Errorf("Database = %q, want mentorship", cfg.Database)
	}
	if got := cfg.RuntimeParams["search_path"]; got != SearchPath {
		t.Errorf("search_path = %q, want %q", got, SearchPath)
	}
}

// The reason this helper exists: in libpq keyword/value syntax these passwords
// are mangled by the parser, so the value reaching Postgres differs from the
// secret and authentication fails. Assigning the field directly must preserve
// them byte for byte.
func TestConnConfigFromEnvPasswordNotParsed(t *testing.T) {
	passwords := []string{
		`back\\slash`,
		`single'quote`,
		`space and = equals`,
		`trailing\`,
		`p@ss w'rd\\with=all`,
	}
	for _, want := range passwords {
		t.Run(want, func(t *testing.T) {
			clearDBEnv(t)
			t.Setenv("DB_HOST", "rds.example.com")
			t.Setenv("DB_USER", "mentorship")
			t.Setenv("DB_PASSWORD", want)
			t.Setenv("DB_NAME", "mentorship")

			cfg, err := ConnConfigFromEnv()
			if err != nil {
				t.Fatalf("ConnConfigFromEnv: %v", err)
			}
			if cfg.Password != want {
				t.Errorf("Password = %q, want %q — the value was altered in transit", cfg.Password, want)
			}
		})
	}
}

func TestConnConfigFromEnvDefaultPort(t *testing.T) {
	clearDBEnv(t)
	t.Setenv("DB_HOST", "rds.example.com")
	t.Setenv("DB_USER", "mentorship")
	t.Setenv("DB_PASSWORD", "s3cret")
	t.Setenv("DB_NAME", "mentorship")

	cfg, err := ConnConfigFromEnv()
	if err != nil {
		t.Fatalf("ConnConfigFromEnv: %v", err)
	}
	if cfg.Port != 5432 {
		t.Errorf("Port = %d, want the 5432 default", cfg.Port)
	}
}

func TestConnConfigFromEnvInvalidPort(t *testing.T) {
	for _, port := range []string{"not-a-number", "0", "99999", "-1"} {
		t.Run(port, func(t *testing.T) {
			clearDBEnv(t)
			t.Setenv("DB_HOST", "rds.example.com")
			t.Setenv("DB_PORT", port)
			t.Setenv("DB_USER", "mentorship")
			t.Setenv("DB_PASSWORD", "s3cret")
			t.Setenv("DB_NAME", "mentorship")

			if _, err := ConnConfigFromEnv(); err == nil {
				t.Fatalf("expected an error for DB_PORT=%q, got nil", port)
			}
		})
	}
}

// A partial discrete configuration must fail loudly rather than fall through to
// DATABASE_DSN, which could point somewhere entirely different.
func TestConnConfigFromEnvPartialDiscreteDoesNotFallBack(t *testing.T) {
	clearDBEnv(t)
	t.Setenv("DB_HOST", "rds.example.com")
	t.Setenv("DATABASE_DSN", "postgres://someone@localhost:5432/other")

	_, err := ConnConfigFromEnv()
	if err == nil {
		t.Fatal("expected an error when DB_HOST is set but the rest are missing")
	}
	for _, want := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name the missing variable %s", err, want)
		}
	}
}

func TestConnConfigFromEnvDSNFallback(t *testing.T) {
	clearDBEnv(t)
	t.Setenv("DATABASE_DSN", "postgres://mentorship:s3cret@localhost:5432/mentorship?sslmode=disable")

	cfg, err := ConnConfigFromEnv()
	if err != nil {
		t.Fatalf("ConnConfigFromEnv: %v", err)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want localhost", cfg.Host)
	}
	if cfg.Database != "mentorship" {
		t.Errorf("Database = %q, want mentorship", cfg.Database)
	}
	if got := cfg.RuntimeParams["search_path"]; got != SearchPath {
		t.Errorf("search_path = %q, want %q", got, SearchPath)
	}
}

// The discrete variables are the cluster's configuration and must take priority
// over a stale or inherited DATABASE_DSN.
func TestConnConfigFromEnvDiscreteWinsOverDSN(t *testing.T) {
	clearDBEnv(t)
	t.Setenv("DB_HOST", "rds.example.com")
	t.Setenv("DB_USER", "mentorship")
	t.Setenv("DB_PASSWORD", "s3cret")
	t.Setenv("DB_NAME", "mentorship")
	t.Setenv("DATABASE_DSN", "postgres://someone@localhost:5432/other")

	cfg, err := ConnConfigFromEnv()
	if err != nil {
		t.Fatalf("ConnConfigFromEnv: %v", err)
	}
	if cfg.Host != "rds.example.com" {
		t.Errorf("Host = %q, want the discrete DB_HOST to win", cfg.Host)
	}
}

func TestConnConfigFromEnvNothingSet(t *testing.T) {
	clearDBEnv(t)

	if _, err := ConnConfigFromEnv(); err == nil {
		t.Fatal("expected an error when neither the DB_* variables nor DATABASE_DSN are set")
	}
}
