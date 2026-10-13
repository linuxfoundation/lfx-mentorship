// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"testing"
)

// The project count is distinct LF project UIDs across published programs:
// a shared UID counts once, and a NULL UID or an unpublished program does not count.
func TestPlatformSummaryRepository_Summary_ProjectCount(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	const projectA, projectB, projectC = "project-a", "project-b", "project-c"
	for _, p := range []struct {
		slug, status string
		projectUID   any
	}{
		{"summary-a1", "published", projectA},
		{"summary-a2", "published", projectA},
		{"summary-b", "published", projectB},
		{"summary-no-project", "published", nil},
		{"summary-pending", "pending", projectC},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO programs (id, name, slug, status, lf_project_uid)
			VALUES (gen_random_uuid(), $1, $1, $2, $3)`, p.slug, p.status, p.projectUID); err != nil {
			t.Fatalf("seed program %s: %v", p.slug, err)
		}
	}

	got, err := NewPlatformSummaryRepository(pool).Summary(ctx)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.ProjectCount != 2 {
		t.Errorf("ProjectCount = %d; want 2", got.ProjectCount)
	}
	if got.ProgramCount != 4 {
		t.Errorf("ProgramCount = %d; want 4", got.ProgramCount)
	}
}
