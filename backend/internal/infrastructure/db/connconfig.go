// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// SearchPath is the schema search path every connection uses. Mentorship tables
// live in the "mentorship" schema (migration 001) rather than "public" because
// this service shares an RDS instance with the other LFX services; "public"
// stays on the path for extensions such as pgcrypto.
const SearchPath = "mentorship,public"

// discreteEnvVars are the per-component connection variables, populated in the
// cluster from the ESO-managed RDS secret.
var discreteEnvVars = []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME"}

// ConnConfigFromEnv builds a *pgx.ConnConfig from the environment.
//
// It prefers the discrete DB_HOST / DB_PORT / DB_USER / DB_PASSWORD / DB_NAME
// variables and assigns them to struct fields directly, so no value is ever
// parsed out of a string. This matters for the password specifically: libpq
// keyword/value syntax ("host=... password=...") treats backslash as an escape
// character, so a password containing "\\" or "'" reaches Postgres altered and
// authentication fails. Assembling a DSN by interpolation is therefore unsafe
// for any credential the service does not control.
//
// DATABASE_DSN remains supported for local development and CI, where a
// postgres:// URL is convenient, and is used when the discrete variables are
// absent. If both are set the discrete variables win.
func ConnConfigFromEnv() (*pgx.ConnConfig, error) {
	if hasDiscreteEnv() {
		return connConfigFromDiscreteEnv()
	}

	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		return nil, fmt.Errorf("database configuration missing: set the discrete DB_* variables (%v) or DATABASE_DSN", discreteEnvVars)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_DSN: %w", err)
	}
	cfg.RuntimeParams["search_path"] = SearchPath
	return cfg, nil
}

// hasDiscreteEnv reports whether the discrete DB_* variables are in use. DB_HOST
// alone is the trigger so that a partial configuration is reported as an error
// rather than silently falling back to DATABASE_DSN and connecting somewhere
// unintended.
func hasDiscreteEnv() bool {
	return os.Getenv("DB_HOST") != ""
}

func connConfigFromDiscreteEnv() (*pgx.ConnConfig, error) {
	var missing []string
	for _, key := range discreteEnvVars {
		if key == "DB_PORT" {
			continue // optional; defaults below
		}
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("DB_HOST is set, so the discrete database variables are in use, but %v are empty", missing)
	}

	port := uint16(5432)
	if raw := os.Getenv("DB_PORT"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("DB_PORT must be a port number, got %q: %w", raw, err)
		}
		if parsed == 0 {
			return nil, fmt.Errorf("DB_PORT must be a port number, got %q", raw)
		}
		port = uint16(parsed)
	}

	sslMode := os.Getenv("DB_SSLMODE")
	if sslMode == "" {
		sslMode = "require"
	}

	// Built through ParseConfig so pgx applies its own defaults and TLS setup,
	// then overridden field by field. Only non-credential values are formatted
	// into the string; the password is assigned directly below.
	//
	// The host and port MUST be part of the parsed string. pgx derives
	// TLSConfig from the host it sees at parse time — with no host it assumes a
	// Unix socket, which never gets TLS, and leaves TLSConfig nil regardless of
	// sslmode. Assigning cfg.Host afterwards does not re-run that derivation, so
	// a "require" connection to RDS would silently go out in plaintext. Passing
	// the host here also gives verify-full the correct SNI server name. Neither
	// value is a credential, so this does not reintroduce the DSN-parsing hazard
	// that this function exists to avoid.
	host := os.Getenv("DB_HOST")
	if strings.ContainsAny(host, " \t\r\n'\\") {
		return nil, fmt.Errorf("DB_HOST contains whitespace or a quoting character and cannot be used: %q", host)
	}
	cfg, err := pgx.ParseConfig(fmt.Sprintf("host=%s port=%d sslmode=%s", host, port, sslMode))
	if err != nil {
		return nil, fmt.Errorf("build database config (DB_HOST=%q, DB_PORT=%d, DB_SSLMODE=%q): %w", host, port, sslMode, err)
	}
	// ParseConfig is authoritative for Host now, but assert it round-tripped so
	// a future change to the format string cannot silently retarget the
	// connection.
	if cfg.Host != host {
		return nil, fmt.Errorf("DB_HOST %q was altered to %q while building the connection config", host, cfg.Host)
	}
	cfg.User = os.Getenv("DB_USER")
	cfg.Password = os.Getenv("DB_PASSWORD")
	cfg.Database = os.Getenv("DB_NAME")
	cfg.RuntimeParams["search_path"] = SearchPath
	return cfg, nil
}
