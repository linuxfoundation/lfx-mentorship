// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func str(v string) *string { return &v }

// The backfill's repository methods: the list pages incomplete signed-in rows
// by a (created_on, id) cursor, and the update fills only columns still NULL.
func TestUserBackfillIntegration(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	repo := NewUserRepository(pool)
	const (
		ada   = "00000000-0000-0000-0000-0000000000b1"
		grace = "00000000-0000-0000-0000-0000000000b2"
		linus = "00000000-0000-0000-0000-0000000000b3"
		whole = "00000000-0000-0000-0000-0000000000b4"
		noLF  = "00000000-0000-0000-0000-0000000000b5"
	)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, stmt := range []string{
		`INSERT INTO users (id, lfid, created_on) VALUES ($1, 'ada', $2)`,
		`INSERT INTO users (id, lfid, email, name, created_on) VALUES ($1, 'grace', 'grace@example.org', 'Grace', $2)`,
		`INSERT INTO users (id, lfid, created_on) VALUES ($1, 'linus', $2)`,
		`INSERT INTO users (id, lfid, email, name, given_name, family_name, avatar_url, created_on) VALUES ($1, 'whole', 'w@example.org', 'W', 'W', 'W', 'https://p/w', $2)`,
		`INSERT INTO users (id, email, created_on) VALUES ($1, 'legacy@example.org', $2)`,
	} {
		id := []string{ada, grace, linus, whole, noLF}[i]
		if _, err := pool.Exec(ctx, stmt, id, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	t.Run("legacy rows with NULL timestamps are read and ordered first", func(t *testing.T) {
		const legacy = "00000000-0000-0000-0000-0000000000b0"
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, created_on, updated_on) VALUES ($1, 'legacy', NULL, NULL)`, legacy); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, legacy) })

		first, err := repo.ListIncompleteIdentities(ctx, nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != 1 || first[0].ID != legacy || !first[0].CreatedOn.Equal(time.Unix(0, 0)) {
			t.Fatalf("first page = %v (created %v); want the NULL-timestamp row first, read as the epoch", ids(first), first[0].CreatedOn)
		}
		// Crossing the page boundary from the legacy row must reach the dated rows.
		cursor := UserCursor{CreatedOn: first[0].CreatedOn, ID: first[0].ID}
		next, err := repo.ListIncompleteIdentities(ctx, &cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(next) != 1 || next[0].ID != ada {
			t.Fatalf("after legacy cursor = %v; want ada", ids(next))
		}
		got, err := repo.FillMissingIdentity(ctx, legacy, models.UserUpdateInput{Name: str("Legacy")})
		if err != nil || *got.Name != "Legacy" || got.UpdatedOn.IsZero() {
			t.Fatalf("fill on a NULL-timestamp row: %+v %v; want it written and scanned", got, err)
		}
	})

	t.Run("list pages incomplete signed-in rows oldest first", func(t *testing.T) {
		first, err := repo.ListIncompleteIdentities(ctx, nil, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != 2 || first[0].ID != ada || first[1].ID != grace {
			t.Fatalf("first page = %v; want ada, grace", ids(first))
		}
		cursor := UserCursor{CreatedOn: first[1].CreatedOn, ID: first[1].ID}
		parsed, err := ParseUserCursor(cursor.String())
		if err != nil || !parsed.CreatedOn.Equal(cursor.CreatedOn) || parsed.ID != cursor.ID {
			t.Fatalf("cursor %q does not round-trip: %v %+v", cursor, err, parsed)
		}
		rest, err := repo.ListIncompleteIdentities(ctx, &parsed, 10)
		if err != nil {
			t.Fatal(err)
		}
		// linus only: whole is complete and the legacy row has no LFID.
		if len(rest) != 1 || rest[0].ID != linus {
			t.Fatalf("after cursor = %v; want linus only", ids(rest))
		}
	})

	t.Run("update fills only NULL columns and keeps what was stored meanwhile", func(t *testing.T) {
		// Grace edited her given name between the backfill's read and its write.
		if _, err := pool.Exec(ctx, `UPDATE users SET given_name = 'Edited' WHERE id = $1`, grace); err != nil {
			t.Fatal(err)
		}
		got, err := repo.FillMissingIdentity(ctx, grace, models.UserUpdateInput{
			Email: str("dir@example.org"), Name: str("Directory"), GivenName: str("Dir"), FamilyName: str("Hopper"), AvatarURL: str("https://p/grace"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if *got.Email != "grace@example.org" || *got.Name != "Grace" || *got.GivenName != "Edited" || *got.FamilyName != "Hopper" || *got.AvatarURL != "https://p/grace" {
			t.Fatalf("after fill: %+v; stored email, name and the concurrent given_name edit must win; family_name and avatar filled", got)
		}
	})

	t.Run("an email another row holds is refused, not taken", func(t *testing.T) {
		_, err := repo.FillMissingIdentity(ctx, ada, models.UserUpdateInput{Email: str("grace@example.org"), Name: str("Ada")})
		if !errors.Is(err, domain.ErrEmailInUse) {
			t.Fatalf("err = %v; want ErrEmailInUse", err)
		}
		var owner string
		if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = 'grace@example.org'`).Scan(&owner); err != nil || owner != grace {
			t.Fatalf("grace@example.org owner = %q (%v); want grace untouched", owner, err)
		}
		var adaName *string
		if err := pool.QueryRow(ctx, `SELECT name FROM users WHERE id = $1`, ada).Scan(&adaName); err != nil || adaName != nil {
			t.Fatalf("ada name = %v (%v); the failed statement must write nothing", adaName, err)
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		if _, err := repo.FillMissingIdentity(ctx, "00000000-0000-0000-0000-0000000000ff", models.UserUpdateInput{Name: str("x")}); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("err = %v; want ErrUserNotFound", err)
		}
	})
}

func ids(users []*models.User) []string {
	out := make([]string, len(users))
	for i, u := range users {
		out[i] = u.ID
	}
	return out
}
