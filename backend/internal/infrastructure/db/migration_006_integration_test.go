// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
)

// Migration 006 closes the prod programs the legacy import left in review: it
// hides the legacy "hide" programs and rejects the stale pending ones, listed
// by ID. Each statement re-checks the status, and the rejection the updated_on
// cutoff, so a program decided or edited after the IDs were taken stays as it is.
func TestMigration006ClosesOnlyListedStalePendingPrograms(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	m := migrator(t)

	const (
		hidden    = "8a8708df-72ee-4799-932e-9e6fe606bfc9" // listed as legacy "hide"
		stale     = "00465abe-5c68-4990-8b02-d261b3970949" // listed for rejection
		edited    = "0161ed06-7d46-415f-9874-491c7fcef69b" // listed, but edited in 2026
		published = "05497ec6-0542-4ff7-ad3f-6c6d292cb7fd" // listed, but approved meanwhile
		unlisted  = "00000000-0000-0000-0000-0000000006a1"
	)
	ids := []string{hidden, stale, edited, published, unlisted}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM programs WHERE id = ANY($1)`, ids); err != nil {
			t.Errorf("delete seeded programs: %v", err)
		}
		// Leave the database at the current schema for the rest of the suite.
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			t.Errorf("restore schema: %v", err)
		}
	})

	if err := m.Migrate(5); err != nil {
		t.Fatalf("step back to schema 5: %v", err)
	}
	old := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO programs (id, name, slug, status, updated_on) VALUES
		($1, 'Hidden', 'm006-hidden', 'pending', $6),
		($2, 'Stale', 'm006-stale', 'pending', $6),
		($3, 'Edited', 'm006-edited', 'pending', $7),
		($4, 'Published', 'm006-published', 'published', $6),
		($5, 'Unlisted', 'm006-unlisted', 'pending', $6)`,
		hidden, stale, edited, published, unlisted, old, recent); err != nil {
		t.Fatalf("seed schema-5 programs: %v", err)
	}

	if err := m.Migrate(6); err != nil {
		t.Fatalf("apply 006: %v", err)
	}

	want := map[string]string{
		hidden:    "hidden",
		stale:     "rejected",
		edited:    "pending",
		published: "published",
		unlisted:  "pending",
	}
	for id, status := range want {
		var got string
		if err := pool.QueryRow(ctx, `SELECT status FROM programs WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != status {
			t.Errorf("%s: status = %q; want %q", id, got, status)
		}
	}
}
