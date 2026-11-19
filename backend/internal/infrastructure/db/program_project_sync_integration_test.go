// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// SyncProjectMetadata writes Project Service's slug, name and logo to the programs of one
// project that differ from it, queues exactly those programs for re-indexing with the new
// values, and leaves the project's current programs and other projects alone (#374).
func TestProgramRepositoryIntegration_SyncProjectMetadata(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	repo := NewProgramRepository(pool)

	const (
		project = "00000000-0000-0000-0000-000000000901"
		other   = "00000000-0000-0000-0000-000000000902"
		stale   = "00000000-0000-0000-0000-000000000911"
		current = "00000000-0000-0000-0000-000000000912"
		foreign = "00000000-0000-0000-0000-000000000913"
		noProj  = "00000000-0000-0000-0000-000000000914"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO programs (id, lf_project_uid, lf_project_slug, lf_project_name, lf_project_logo_url, name, slug, status) VALUES
			($1, $5, 'old-slug', 'Old Name', 'https://logo/old.png', 'Stale', 'stale', 'published'),
			($2, $5, 'new-slug', 'New Name', NULL, 'Current', 'current', 'published'),
			($3, $6, 'old-slug', 'Old Name', 'https://logo/old.png', 'Foreign', 'foreign', 'published'),
			($4, NULL, NULL, NULL, NULL, 'No Project', 'no-project', 'pending')`,
		stale, current, foreign, noProj, project, other); err != nil {
		t.Fatal(err)
	}

	listed, err := repo.ListProgramProjectMetadata(ctx)
	if err != nil {
		t.Fatalf("ListProgramProjectMetadata: %v", err)
	}
	var listedIDs []string
	for _, p := range listed {
		listedIDs = append(listedIDs, p.ProgramID)
	}
	if want := []string{stale, current, foreign}; !slices.Equal(listedIDs, want) {
		t.Fatalf("listed %v; want %v, ordered by project and without the program that has no project", listedIDs, want)
	}

	ids, err := repo.SyncProjectMetadata(ctx, project, models.ProjectMetadata{Slug: "new-slug", Name: "New Name"})
	if err != nil {
		t.Fatalf("SyncProjectMetadata: %v", err)
	}
	if !slices.Equal(ids, []string{stale}) {
		t.Fatalf("changed %v; want only the stale program", ids)
	}

	type stored struct{ slug, name, logo *string }
	read := func(id string) stored {
		var s stored
		if err := pool.QueryRow(ctx, `SELECT lf_project_slug, lf_project_name, lf_project_logo_url FROM programs WHERE id = $1`, id).Scan(&s.slug, &s.name, &s.logo); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := read(stale); *s.slug != "new-slug" || *s.name != "New Name" || s.logo != nil {
		t.Errorf("stale program = %v %v %v; want new-slug, New Name and the logo cleared", *s.slug, *s.name, s.logo)
	}
	if s := read(foreign); *s.name != "Old Name" {
		t.Errorf("another project's program was renamed to %q", *s.name)
	}

	var queued []string
	rows, err := pool.Query(ctx, `SELECT object_uid, data FROM index_outbox WHERE object_type = 'mentorship_program' ORDER BY object_uid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			t.Fatal(err)
		}
		queued = append(queued, id)
		var doc ProgramIndexDocument
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		if doc.ProjectName == nil || *doc.ProjectName != "New Name" || doc.ProjectSlug == nil || *doc.ProjectSlug != "new-slug" {
			t.Errorf("index document for %s carries project %v / %v; want the new name and slug", id, doc.ProjectName, doc.ProjectSlug)
		}
	}
	if !slices.Equal(queued, []string{stale}) {
		t.Fatalf("queued for re-indexing %v; want only the changed program", queued)
	}

	again, err := repo.SyncProjectMetadata(ctx, project, models.ProjectMetadata{Slug: "new-slug", Name: "New Name"})
	if err != nil || len(again) != 0 {
		t.Fatalf("second sync changed %v (err %v); want nothing, the programs are current", again, err)
	}
}
