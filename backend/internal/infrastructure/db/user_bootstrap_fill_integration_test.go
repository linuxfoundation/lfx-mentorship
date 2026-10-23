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

func ptr(v string) *string { return &v }

// BootstrapByLFID applies the sign-in's own values over stored ones and the
// directory fill only to columns still NULL, in one statement, so an edit made
// while the directory lookup was in flight survives (#362 review).
func TestUserBootstrapFillIntegration(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	repo := NewUserRepository(pool)
	const alice, bob = "00000000-0000-0000-0000-0000000000c1", "00000000-0000-0000-0000-0000000000c2"
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid) VALUES ($1, 'alice'), ($2, 'bob')`, alice, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET email = 'bob@example.org' WHERE id = $1`, bob); err != nil {
		t.Fatal(err)
	}

	t.Run("fill does not replace a value stored after the read", func(t *testing.T) {
		// Bootstrap read alice with a NULL name; she saves one before the write lands.
		if _, err := pool.Exec(ctx, `UPDATE users SET name = 'Edited', avatar_url = 'https://p/edited' WHERE id = $1`, alice); err != nil {
			t.Fatal(err)
		}
		got, err := repo.BootstrapByLFID(ctx,
			models.UserCreateInput{LFID: ptr("alice")},
			models.UserIdentityFill{Name: ptr("Directory"), GivenName: ptr("Alice"), AvatarURL: ptr("https://p/dir")})
		if err != nil {
			t.Fatal(err)
		}
		if *got.Name != "Edited" || *got.AvatarURL != "https://p/edited" || *got.GivenName != "Alice" {
			t.Fatalf("after fill: name=%v avatar=%v given=%v; want the edits kept and only given_name filled", *got.Name, *got.AvatarURL, *got.GivenName)
		}
	})

	t.Run("input still replaces, fill still only fills, in the same write", func(t *testing.T) {
		got, err := repo.BootstrapByLFID(ctx,
			models.UserCreateInput{LFID: ptr("alice"), Name: ptr("From Token")},
			models.UserIdentityFill{Name: ptr("Directory"), FamilyName: ptr("Liddell")})
		if err != nil {
			t.Fatal(err)
		}
		if *got.Name != "From Token" || *got.FamilyName != "Liddell" {
			t.Fatalf("name=%v family=%v; want the token name to replace and the family name to fill", *got.Name, *got.FamilyName)
		}
	})

	t.Run("a fill email never takes another row's address", func(t *testing.T) {
		_, err := repo.BootstrapByLFID(ctx, models.UserCreateInput{LFID: ptr("alice")}, models.UserIdentityFill{Email: ptr("bob@example.org")})
		if !errors.Is(err, domain.ErrEmailInUse) {
			t.Fatalf("err = %v; want ErrEmailInUse", err)
		}
		var owner string
		if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = 'bob@example.org'`).Scan(&owner); err != nil || owner != bob {
			t.Fatalf("bob@example.org owner = %q (%v); want bob untouched", owner, err)
		}
	})

	t.Run("a token email still reclaims the address", func(t *testing.T) {
		got, err := repo.BootstrapByLFID(ctx, models.UserCreateInput{LFID: ptr("alice"), Email: ptr("bob@example.org")}, models.UserIdentityFill{})
		if err != nil {
			t.Fatal(err)
		}
		var bobEmail *string
		if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, bob).Scan(&bobEmail); err != nil {
			t.Fatal(err)
		}
		if *got.Email != "bob@example.org" || bobEmail != nil {
			t.Fatalf("alice email=%v, bob email=%v; want the verified token email moved to alice", *got.Email, bobEmail)
		}
	})

	t.Run("a new row takes the fill", func(t *testing.T) {
		got, err := repo.BootstrapByLFID(ctx, models.UserCreateInput{LFID: ptr("carol")}, models.UserIdentityFill{Name: ptr("Carol"), Email: ptr("carol@example.org")})
		if err != nil {
			t.Fatal(err)
		}
		if *got.Name != "Carol" || *got.Email != "carol@example.org" {
			t.Fatalf("new row = %+v; want the fill applied on insert", got)
		}
	})
}
