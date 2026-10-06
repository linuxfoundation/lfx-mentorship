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
	_, err := NewUserRepository(pool).UpsertByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: &email})
	if !errors.Is(err, domain.ErrEmailInUse) {
		t.Fatalf("err = %v; want ErrEmailInUse", err)
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
