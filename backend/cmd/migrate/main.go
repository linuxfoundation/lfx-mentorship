// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Command migrate applies pending db/migrations to DATABASE_DSN. It is run as
// a Helm pre-install/pre-upgrade hook Job so schema changes land before the
// new application image starts serving traffic.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/db/migrations"
)

func main() {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		log.Fatal("DATABASE_DSN is required")
	}

	// Parsed with pgx rather than net/url: DATABASE_DSN is a postgres:// URL
	// locally/in CI but a libpq keyword/value string ("host=... user=...") in
	// the cluster, where it is assembled from the ESO-managed RDS secret
	// components (see lfx-v2-argocd values), which net/url can't parse.
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("parse DATABASE_DSN: %v", err)
	}
	// Fail fast rather than queue behind live traffic waiting for an
	// ACCESS EXCLUSIVE lock: a blocked DDL statement holds a lock queue that
	// stalls every subsequent query on the table.
	connConfig.RuntimeParams["lock_timeout"] = "5s"
	connConfig.RuntimeParams["statement_timeout"] = "30s"

	sqlDB := stdlib.OpenDB(*connConfig)
	defer sqlDB.Close()

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{
		// Pin golang-migrate's version-tracking table to "public". The driver
		// otherwise defaults to CURRENT_SCHEMA() — the first entry on the
		// connection's search_path — which under Postgres' default
		// "$user",public resolves to a schema named after the DB user when one
		// exists. That would put the tracking table somewhere the next run may
		// not resolve, silently re-applying every migration from scratch.
		MigrationsTable:       `"public"."schema_migrations"`,
		MigrationsTableQuoted: true,
	})
	if err != nil {
		log.Fatalf("init postgres driver: %v", err)
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		log.Fatalf("load embedded migrations: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		log.Fatalf("init migrator: %v", err)
	}

	// One-time bootstrap for a database whose schema was already brought up to
	// some migration N by hand but has no schema_migrations row recording it.
	// Run manually once per such environment: `migrate force N`. Never part of
	// the automatic pre-install/pre-upgrade hook path.
	switch len(os.Args) {
	case 1:
		// no subcommand — normal m.Up() path below.
	case 3:
		if os.Args[1] != "force" {
			log.Fatal("usage: migrate force <version>")
		}
		version, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatalf("invalid version %q: %v", os.Args[2], err)
		}
		if err := m.Force(version); err != nil {
			log.Fatalf("force version: %v", err)
		}
		fmt.Printf("schema_migrations forced to version %d\n", version)
		return
	default:
		log.Fatal("usage: migrate force <version>")
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("apply migrations: %v", err)
	}

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		log.Fatalf("read schema version: %v", err)
	}
	fmt.Printf("migrations applied: schema version %d (dirty=%v)\n", version, dirty)
}
