// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/db/migrations"
)

// migrator opens golang-migrate on the test database, tracking versions in
// public.schema_migrations as cmd/migrate does.
func migrator(t *testing.T) *migrate.Migrate {
	t.Helper()
	config, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	connConfig := config.ConnConfig
	connConfig.RuntimeParams["search_path"] = "mentorship,public"
	sqlDB := stdlib.OpenDB(*connConfig)
	t.Cleanup(func() { _ = sqlDB.Close() })
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{MigrationsTable: `"public"."schema_migrations"`, MigrationsTableQuoted: true})
	if err != nil {
		t.Fatal(err)
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Migration 005 repairs users rows that the legacy importer left without a
// timestamp and forbids new ones (#364). The repair relies on the
// set_updated_on trigger for updated_on, so it is checked against a real
// database: step the schema back to 4, seed the three NULL shapes, apply 005.
func TestMigration005RepairsAndForbidsNullUserTimestamps(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	m := migrator(t)
	t.Cleanup(func() {
		// Leave the database at the current schema for the rest of the suite.
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			t.Errorf("restore schema: %v", err)
		}
	})

	if err := m.Migrate(4); err != nil {
		t.Fatalf("step back to schema 4: %v", err)
	}
	const (
		none        = "00000000-0000-0000-0000-0000000000d1"
		createdOnly = "00000000-0000-0000-0000-0000000000d2"
		updatedOnly = "00000000-0000-0000-0000-0000000000d3"
	)
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, created_on, updated_on) VALUES
		($1, 'ts-none', NULL, NULL), ($2, 'ts-created', $4, NULL), ($3, 'ts-updated', NULL, $5)`,
		none, createdOnly, updatedOnly, created, updated); err != nil {
		t.Fatalf("seed schema-4 rows: %v", err)
	}
	before := time.Now().Add(-time.Minute)

	if err := m.Migrate(5); err != nil {
		t.Fatalf("apply 005: %v", err)
	}

	rows := map[string]struct{ created, updated time.Time }{}
	for _, id := range []string{none, createdOnly, updatedOnly} {
		var c, u sql.NullTime
		if err := pool.QueryRow(ctx, `SELECT created_on, updated_on FROM users WHERE id = $1`, id).Scan(&c, &u); err != nil {
			t.Fatal(err)
		}
		if !c.Valid || !u.Valid {
			t.Fatalf("%s still has a NULL timestamp after 005", id)
		}
		rows[id] = struct{ created, updated time.Time }{c.Time, u.Time}
	}
	if !rows[createdOnly].created.Equal(created) {
		t.Errorf("created-only row: created_on = %v; want the stored %v kept", rows[createdOnly].created, created)
	}
	if !rows[updatedOnly].created.Equal(updated) {
		t.Errorf("updated-only row: created_on = %v; want the stored updated_on %v", rows[updatedOnly].created, updated)
	}
	if rows[none].created.Before(before) {
		t.Errorf("no-timestamp row: created_on = %v; want the repair time", rows[none].created)
	}
	for id, r := range rows {
		if r.updated.Before(before) {
			t.Errorf("%s: updated_on = %v; want the repair time from set_updated_on", id, r.updated)
		}
	}

	var nullable string
	if err := pool.QueryRow(ctx, `SELECT string_agg(column_name || '=' || is_nullable, ',' ORDER BY column_name)
		FROM information_schema.columns WHERE table_schema = 'mentorship' AND table_name = 'users' AND column_name IN ('created_on', 'updated_on')`).Scan(&nullable); err != nil {
		t.Fatal(err)
	}
	if nullable != "created_on=NO,updated_on=NO" {
		t.Fatalf("nullability after 005 = %s; want both NOT NULL", nullable)
	}
	_, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, created_on) VALUES ('00000000-0000-0000-0000-0000000000d4', 'ts-explicit-null', NULL)`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23502" {
		t.Fatalf("explicit NULL insert: err = %v; want not-null violation (23502)", err)
	}
}
