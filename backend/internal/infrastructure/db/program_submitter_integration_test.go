// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

func TestProgramRepositoryIntegration_GetSubmitter(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	s := newMentoredSeed(t, pool)
	repo := NewProgramRepository(pool)

	named := s.program("named", "Named Submitter", "pending")
	s.exec(`INSERT INTO users (id, lfid, name, email, avatar_url) VALUES ($1, 'submitter-named', 'Jane Doe', 'jane.doe@example.org', 'https://example.org/jane.png')`, s.id("named-user"))
	s.exec(`UPDATE programs SET lfid = 'submitter-named' WHERE id = $1`, named)

	got, err := repo.GetSubmitter(ctx, named)
	if err != nil {
		t.Fatalf("GetSubmitter: %v", err)
	}
	if got.LFID != "submitter-named" || got.Name == nil || *got.Name != "Jane Doe" ||
		got.Email == nil || *got.Email != "jane.doe@example.org" ||
		got.AvatarURL == nil || *got.AvatarURL != "https://example.org/jane.png" {
		t.Errorf("got %+v; want the submitter's name, email and avatar", got)
	}

	given := s.program("given", "Given Name Submitter", "pending")
	s.exec(`INSERT INTO users (id, lfid, name, given_name, family_name) VALUES ($1, 'submitter-given', ' ', 'Ada', 'Lovelace')`, s.id("given-user"))
	s.exec(`UPDATE programs SET lfid = 'submitter-given' WHERE id = $1`, given)
	if got, err := repo.GetSubmitter(ctx, given); err != nil || got.Name == nil || *got.Name != "Ada Lovelace" || got.Email != nil {
		t.Errorf("blank name = %+v, %v; want the given and family names and no email", got, err)
	}

	orphan := s.program("orphan", "No User Row", "pending")
	s.exec(`UPDATE programs SET lfid = 'submitter-without-row' WHERE id = $1`, orphan)
	if got, err := repo.GetSubmitter(ctx, orphan); err != nil || got.LFID != "submitter-without-row" || got.Name != nil || got.Email != nil {
		t.Errorf("no users row = %+v, %v; want the LFID alone", got, err)
	}

	unowned := s.program("unowned", "No Submitter", "pending")
	if _, err := repo.GetSubmitter(ctx, unowned); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("no lfid err = %v; want ErrUserNotFound", err)
	}

	if _, err := repo.GetSubmitter(ctx, s.id("missing")); !errors.Is(err, domain.ErrProgramNotFound) {
		t.Errorf("unknown program err = %v; want ErrProgramNotFound", err)
	}
}
