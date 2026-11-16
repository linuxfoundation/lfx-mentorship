// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestUserRepository_UpsertByLFID_EmailInUse(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES (gen_random_uuid(), 'legacy.inuse@example.org')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE email LIKE '%.inuse@example.org'`)
	})
	lfid, email := "inuse_lfid", "legacy.inuse@example.org"
	if _, err := NewUserRepository(pool).UpsertByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: &email}); !errors.Is(err, domain.ErrEmailInUse) {
		t.Fatalf("UpsertByLFID err = %v; want ErrEmailInUse", err)
	}
}

// An imported row without an LFID that still holds the verified token email gives it up like any
// other stale holder, so it cannot block the owner's sign-in.
func TestUserRepository_BootstrapByLFID_ReleasesEmailFromImportedRow(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	const imported = "00000000-0000-0000-0000-0000000000b1"
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1, 'imported.release@example.org')`, imported); err != nil {
		t.Fatal(err)
	}
	lfid, email := "imported_owner", "imported.release@example.org"
	u, err := NewUserRepository(pool).BootstrapByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == imported || u.Email == nil || *u.Email != email {
		t.Fatalf("user = %+v; want a new row with email %q", u, email)
	}
	var importedEmail *string
	if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, imported).Scan(&importedEmail); err != nil {
		t.Fatal(err)
	}
	if importedEmail != nil {
		t.Fatalf("imported row email = %q; want NULL", *importedEmail)
	}
}

// Several first sign-ins for one LFID arrive together (Self Serve sends two PUT /v1/me on a third
// of first visits). All find no row; without serialisation the later inserts fail on
// users_email_key before the lfid arbiter applies. Without the per-LFID lock this fails in more
// than half of the iterations.
func TestUserRepository_BootstrapByLFID_ConcurrentFirstSignIns(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	repo := NewUserRepository(pool)
	for i := 0; i < 100; i++ {
		lfid, email := fmt.Sprintf("race_%d", i), fmt.Sprintf("race_%d@example.org", i)
		const callers = 4
		var wg sync.WaitGroup
		errs := make(chan error, callers)
		for c := 0; c < callers; c++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := repo.BootstrapByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: &email})
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("iteration %d: %v", i, err)
			}
		}
		var rows int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE lfid = $1 OR email = $2`, lfid, email).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 1 {
			t.Fatalf("iteration %d: rows for the LFID or email = %d; want 1", i, rows)
		}
	}
}

// A verified email held by another LF account's row is stale: bootstrap takes it over and
// clears it there, so a squatted address cannot block the owner's first sign-in.
func TestUserRepository_BootstrapByLFID_ReleasesEmailFromAnotherAccount(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, lfid) VALUES (gen_random_uuid(), 'owner.release@example.org', 'squatter_lfid')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lfid IN ('squatter_lfid', 'owner_lfid')`)
	})
	lfid, email := "owner_lfid", "owner.release@example.org"
	u, err := NewUserRepository(pool).BootstrapByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	if u.Email == nil || *u.Email != email {
		t.Fatalf("email = %v; want %q", u.Email, email)
	}
	var squatter *string
	if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE lfid = 'squatter_lfid'`).Scan(&squatter); err != nil {
		t.Fatal(err)
	}
	if squatter != nil {
		t.Fatalf("squatter email = %q; want NULL", *squatter)
	}
}

// A sign-in with no verified email keeps the stored one, as UpsertByLFID does: clearing it would
// cut the user off from notifications and email invites.
func TestUserRepository_BootstrapByLFID_KeepsEmailWithoutClaim(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lfid IN ('noclaim_upsert', 'noclaim_bootstrap')`)
	})
	repo := NewUserRepository(pool)
	for lfid, upsert := range map[string]func(context.Context, models.UserCreateInput) (*models.User, error){
		"noclaim_upsert":    repo.UpsertByLFID,
		"noclaim_bootstrap": repo.BootstrapByLFID,
	} {
		stored := lfid + ".stored@example.org"
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, lfid) VALUES (gen_random_uuid(), $1, $2)`, stored, lfid); err != nil {
			t.Fatal(err)
		}
		u, err := upsert(ctx, models.UserCreateInput{LFID: &lfid})
		if err != nil {
			t.Fatalf("%s: %v", lfid, err)
		}
		if u.Email == nil || *u.Email != stored {
			t.Fatalf("%s: email = %v; want the stored %q", lfid, u.Email, stored)
		}
	}
}

// Login refreshes the avatar from the identity's picture, and a login without one keeps it.
func TestUserRepository_UpsertByLFID_RefreshesAvatar(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lfid = 'avatar_lfid'`)
	})
	repo := NewUserRepository(pool)
	lfid, first, second := "avatar_lfid", "https://auth0.example/first.png", "https://auth0.example/second.png"
	for _, tc := range []struct {
		name   string
		avatar *string
		want   string
	}{
		{"first login", &first, first},
		{"new picture", &second, second},
		{"no picture", nil, second},
	} {
		u, err := repo.UpsertByLFID(ctx, models.UserCreateInput{LFID: &lfid, AvatarURL: tc.avatar})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if u.AvatarURL == nil || *u.AvatarURL != tc.want {
			t.Fatalf("%s: avatar_url = %v; want %q", tc.name, u.AvatarURL, tc.want)
		}
	}
}

func TestUserRepository_SearchCandidates(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, lfid, name) VALUES
			(gen_random_uuid(), 'grace.cand@example.org', 'cand_grace', 'Grace Hopper'),
			(gen_random_uuid(), 'gracie.cand@example.org', 'candxgracie', 'Gracie Other'),
			(gen_random_uuid(), 'fan.cand@example.org', 'candxgracie2', 'Aaron Fan'),
			(gen_random_uuid(), 'nolfid.cand@example.org', NULL, 'Grace Nolfid')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE email LIKE '%.cand@example.org'`)
	})
	repo := NewUserRepository(pool)

	for name, tc := range map[string]struct {
		query string
		want  []string
	}{
		"name substring":         {"hopper", []string{"cand_grace"}},
		"lfid prefix":            {"cand", []string{"candxgracie2", "cand_grace", "candxgracie"}},
		"underscore is literal":  {"cand_", []string{"cand_grace"}},
		"whole email no match":   {"GRACE.cand@example.org", nil},
		"partial email no match": {"cand@example.org", nil},
		"rows without lfid omit": {"nolfid", nil},
		// candxgracie2 sorts first by name, so only the exact-LFID ranking puts candxgracie ahead.
		"exact lfid ranked first": {"candxgracie", []string{"candxgracie", "candxgracie2"}},
	} {
		got, err := repo.SearchCandidates(ctx, tc.query, 10)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var lfids []string
		for _, u := range got {
			lfids = append(lfids, *u.LFID)
		}
		if len(lfids) != len(tc.want) {
			t.Errorf("%s: got %v; want %v", name, lfids, tc.want)
			continue
		}
		for i := range lfids {
			if lfids[i] != tc.want[i] {
				t.Errorf("%s: got %v; want %v", name, lfids, tc.want)
				break
			}
		}
	}
}

// Two accounts holding each other's address bootstrap at once: the row locks are taken in a
// fixed order, so neither sign-in fails with a deadlock and each ends with its own address.
func TestUserRepository_BootstrapByLFID_SwappedAddressesConcurrently(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lfid IN ('swap_a', 'swap_b')`)
	})
	repo := NewUserRepository(pool)
	accounts := map[string]string{"swap_a": "a.swap@example.org", "swap_b": "b.swap@example.org"}
	for i := 0; i < 20; i++ {
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE lfid IN ('swap_a', 'swap_b')`); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, lfid) VALUES
			(gen_random_uuid(), 'b.swap@example.org', 'swap_a'),
			(gen_random_uuid(), 'a.swap@example.org', 'swap_b')`); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan error, len(accounts))
		for lfid, email := range accounts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := repo.BootstrapByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: &email})
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("iteration %d: %v", i, err)
			}
		}
		for lfid, email := range accounts {
			var got *string
			if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE lfid = $1`, lfid).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got == nil || *got != email {
				t.Fatalf("iteration %d: %s email = %v; want %q", i, lfid, got, email)
			}
		}
	}
}

// UpsertByLFIDIfEmailUnchanged writes an email that was looked up before the transaction only
// while the stored one is still what it was then, so a stale directory answer never replaces a
// verified email a concurrent sign-in stored meanwhile. It never takes the address from another row.
func TestUserRepository_UpsertByLFIDIfEmailUnchanged(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	repo := NewUserRepository(pool)
	str := func(v string) *string { return &v }
	emailOf := func(t *testing.T, lfid string) *string {
		t.Helper()
		var email *string
		if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE lfid = $1`, lfid).Scan(&email); err != nil {
			t.Fatal(err)
		}
		return email
	}
	show := func(v *string) string {
		if v == nil {
			return "NULL"
		}
		return *v
	}

	for lfid, tc := range map[string]struct {
		name      string
		row       bool    // the caller has a row when the write lands
		rowEmail  *string // its email then
		unchanged *string // the email read before the lookup
		want      string
	}{
		"guard_same":    {name: "stored email unchanged since the read: replaced", row: true, rowEmail: str("guard_same.old@example.org"), unchanged: str("guard_same.old@example.org"), want: "guard_same.dir@example.org"},
		"guard_newer":   {name: "verified email stored since the read: kept", row: true, rowEmail: str("guard_newer.verified@example.org"), unchanged: str("guard_newer.old@example.org"), want: "guard_newer.verified@example.org"},
		"guard_created": {name: "row created with an email since the read: kept", row: true, rowEmail: str("guard_created.verified@example.org"), want: "guard_created.verified@example.org"},
		"guard_empty":   {name: "row without an email: filled", row: true, want: "guard_empty.dir@example.org"},
		"guard_new":     {name: "no row: created with the email", want: "guard_new.dir@example.org"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.row {
				if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, lfid) VALUES (gen_random_uuid(), $1, $2)`, tc.rowEmail, lfid); err != nil {
					t.Fatal(err)
				}
			}
			directory := lfid + ".dir@example.org"
			u, err := repo.UpsertByLFIDIfEmailUnchanged(ctx, models.UserCreateInput{LFID: &lfid, Email: &directory}, tc.unchanged)
			if err != nil {
				t.Fatal(err)
			}
			if got := emailOf(t, lfid); show(got) != tc.want || show(u.Email) != tc.want {
				t.Fatalf("stored email = %s, returned %s; want %s", show(got), show(u.Email), tc.want)
			}
		})
	}

	t.Run("an address another row holds is refused and both rows keep their email", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, lfid) VALUES
			(gen_random_uuid(), 'guard_holder@example.org', 'guard_holder'),
			(gen_random_uuid(), 'guard_caller@example.org', 'guard_caller')`); err != nil {
			t.Fatal(err)
		}
		lfid := "guard_caller"
		_, err := repo.UpsertByLFIDIfEmailUnchanged(ctx, models.UserCreateInput{LFID: &lfid, Email: str("guard_holder@example.org")}, str("guard_caller@example.org"))
		if !errors.Is(err, domain.ErrEmailInUse) {
			t.Fatalf("err = %v; want ErrEmailInUse", err)
		}
		if holder, caller := show(emailOf(t, "guard_holder")), show(emailOf(t, lfid)); holder != "guard_holder@example.org" || caller != "guard_caller@example.org" {
			t.Fatalf("holder email = %s, caller email = %s; want both unchanged", holder, caller)
		}
	})
}
