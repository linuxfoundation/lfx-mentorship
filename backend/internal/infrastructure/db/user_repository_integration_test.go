// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
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
	repo := NewUserRepository(pool)
	if _, err := repo.UpsertByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: &email}); !errors.Is(err, domain.ErrEmailInUse) {
		t.Fatalf("UpsertByLFID err = %v; want ErrEmailInUse", err)
	}
	// Bootstrap releases an address only from another LF account, never from a legacy row.
	if _, err := repo.BootstrapByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: &email}); !errors.Is(err, domain.ErrEmailInUse) {
		t.Fatalf("BootstrapByLFID err = %v; want ErrEmailInUse", err)
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

// Bootstrap stores only the token's claim: with no claim, an email written before the account
// email was bound to the claim is cleared, while UpsertByLFID keeps the stored one.
func TestUserRepository_BootstrapByLFID_ClearsEmailWithoutClaim(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lfid IN ('noclaim_upsert', 'noclaim_bootstrap')`)
	})
	repo := NewUserRepository(pool)
	for lfid, tc := range map[string]struct {
		upsert func(context.Context, models.UserCreateInput) (*models.User, error)
		want   *string
	}{
		"noclaim_upsert":    {repo.UpsertByLFID, ptrTo("noclaim_upsert.forged@example.org")},
		"noclaim_bootstrap": {repo.BootstrapByLFID, nil},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, lfid) VALUES (gen_random_uuid(), $1, $2)`, lfid+".forged@example.org", lfid); err != nil {
			t.Fatal(err)
		}
		u, err := tc.upsert(ctx, models.UserCreateInput{LFID: &lfid})
		if err != nil {
			t.Fatalf("%s: %v", lfid, err)
		}
		if (u.Email == nil) != (tc.want == nil) || (tc.want != nil && *u.Email != *tc.want) {
			t.Fatalf("%s: email = %v; want %v", lfid, u.Email, tc.want)
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
