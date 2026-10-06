// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"testing"
)

func TestUserRepository_SearchCandidates(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, lfid, name) VALUES
			(gen_random_uuid(), 'grace.cand@example.org', 'cand_grace', 'Grace Hopper'),
			(gen_random_uuid(), 'gracie.cand@example.org', 'candxgracie', 'Gracie Other'),
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
		"name substring":          {"hopper", []string{"cand_grace"}},
		"lfid prefix":             {"cand", []string{"cand_grace", "candxgracie"}},
		"underscore is literal":   {"cand_", []string{"cand_grace"}},
		"whole email no match":    {"GRACE.cand@example.org", nil},
		"partial email no match":  {"cand@example.org", nil},
		"rows without lfid omit":  {"nolfid", nil},
		"exact lfid ranked first": {"candxgracie", []string{"candxgracie"}},
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
