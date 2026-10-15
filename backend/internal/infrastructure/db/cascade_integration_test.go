// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// Deleting a program or an application must, in the same transaction, remove
// the rows, queue the stored files for deletion, and tell FGA and the indexer
// about every descendant. These tests check each side effect against Postgres.

type cascadeFixture struct {
	integrationFixture
	ApplicationID, TaskID, LogoURL, TaskFile string
}

func seedCascadeFixture(t *testing.T, pool *pgxpool.Pool) cascadeFixture {
	t.Helper()
	ctx := context.Background()
	f := cascadeFixture{
		integrationFixture: seedIntegrationFixture(t, pool),
		ApplicationID:      "00000000-0000-0000-0000-000000000601",
		TaskID:             "00000000-0000-0000-0000-000000000602",
		LogoURL:            "https://cdn.example.org/logos/fixture.png",
		TaskFile:           "attachments/fixture/report.pdf",
	}
	if _, err := pool.Exec(ctx, `UPDATE programs SET logo_url = $2 WHERE id = $1`, f.ProgramID, f.LogoURL); err != nil {
		t.Fatalf("set logo: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO applications (id, program_term_id, user_id, role, status, program_term_status)
		VALUES ($1, $2, $3, 'mentee', 'accepted', 'open')`, f.ApplicationID, f.OpenTerm, f.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status, file, application_status, program_term_status)
		VALUES ($1, $2, $3, $4, 'submitted', $5, 'accepted', 'open')`, f.TaskID, f.ApplicationID, f.OpenTerm, f.UserID, f.TaskFile); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	// Side effects from seeding must not be mistaken for the delete's.
	if _, err := pool.Exec(ctx, `TRUNCATE fga_outbox, index_outbox, object_deletions`); err != nil {
		t.Fatalf("clear outboxes: %v", err)
	}
	return f
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func assertFGAMarker(t *testing.T, pool *pgxpool.Pool, objectType, objectUID, operation string) {
	t.Helper()
	if n := countRows(t, pool, `SELECT COUNT(*) FROM fga_outbox WHERE marker_kind = 'object' AND object_type = $1 AND object_uid = $2 AND desired_operation = $3 AND state = 'pending'`, objectType, objectUID, operation); n != 1 {
		t.Errorf("fga markers for %s %s %s = %d; want 1", objectType, objectUID, operation, n)
	}
}

func assertIndexAction(t *testing.T, pool *pgxpool.Pool, objectType, objectUID, action string) {
	t.Helper()
	if n := countRows(t, pool, `SELECT COUNT(*) FROM index_outbox WHERE object_type = $1 AND object_uid = $2 AND action = $3`, objectType, objectUID, action); n != 1 {
		t.Errorf("index %s for %s %s = %d; want 1", action, objectType, objectUID, n)
	}
}

func assertObjectDeletion(t *testing.T, pool *pgxpool.Pool, bucket domain.ObjectBucket, locator string) {
	t.Helper()
	if n := countRows(t, pool, `SELECT COUNT(*) FROM object_deletions WHERE bucket = $1 AND locator = $2 AND state = 'pending'`, string(bucket), locator); n != 1 {
		t.Errorf("object deletions for %s %s = %d; want 1", bucket, locator, n)
	}
}

func TestProgramRepositoryIntegration_DeleteCascadesToDescendants(t *testing.T) {
	pool := integrationPool(t)
	f := seedCascadeFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})

	if err := NewProgramRepository(pool).Delete(ctx, f.ProgramID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	for _, q := range []struct{ table, id string }{
		{"programs", f.ProgramID}, {"program_terms", f.OpenTerm}, {"applications", f.ApplicationID}, {"tasks", f.TaskID},
	} {
		if n := countRows(t, pool, `SELECT COUNT(*) FROM `+q.table+` WHERE id = $1`, q.id); n != 0 {
			t.Errorf("%s still has %s", q.table, q.id)
		}
	}
	assertFGAMarker(t, pool, "mentorship_program", f.ProgramID, "delete_access")
	assertFGAMarker(t, pool, "mentorship_application", f.ApplicationID, "delete_access")
	assertFGAMarker(t, pool, "mentorship_task", f.TaskID, "delete_access")
	assertIndexAction(t, pool, "mentorship_program", f.ProgramID, "deleted")
	assertIndexAction(t, pool, "mentorship_application", f.ApplicationID, "deleted")
	assertIndexAction(t, pool, "mentorship_task", f.TaskID, "deleted")
	assertObjectDeletion(t, pool, domain.ObjectBucketLogos, f.LogoURL)
	assertObjectDeletion(t, pool, domain.ObjectBucketAttachments, f.TaskFile)
	if n := countRows(t, pool, `SELECT COUNT(*) FROM object_deletions`); n != 2 {
		t.Errorf("object deletions = %d; want exactly the logo and the task file", n)
	}

	if err := NewProgramRepository(pool).Delete(ctx, f.ProgramID); !errors.Is(err, domain.ErrProgramNotFound) {
		t.Errorf("second delete err = %v; want ErrProgramNotFound", err)
	}
}

func TestApplicationRepositoryIntegration_DeleteCascadesToTasks(t *testing.T) {
	pool := integrationPool(t)
	f := seedCascadeFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})

	if err := NewApplicationRepository(pool).Delete(ctx, f.ApplicationID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if n := countRows(t, pool, `SELECT COUNT(*) FROM applications WHERE id = $1`, f.ApplicationID); n != 0 {
		t.Error("application still exists")
	}
	if n := countRows(t, pool, `SELECT COUNT(*) FROM tasks WHERE id = $1`, f.TaskID); n != 0 {
		t.Error("task still exists")
	}
	if n := countRows(t, pool, `SELECT COUNT(*) FROM programs WHERE id = $1`, f.ProgramID); n != 1 {
		t.Error("program must survive an application delete")
	}
	assertFGAMarker(t, pool, "mentorship_application", f.ApplicationID, "delete_access")
	assertFGAMarker(t, pool, "mentorship_task", f.TaskID, "delete_access")
	assertIndexAction(t, pool, "mentorship_application", f.ApplicationID, "deleted")
	assertIndexAction(t, pool, "mentorship_task", f.TaskID, "deleted")
	// The program's mentee counts changed, so its document is refreshed, not deleted.
	assertIndexAction(t, pool, "mentorship_program", f.ProgramID, "updated")
	assertObjectDeletion(t, pool, domain.ObjectBucketAttachments, f.TaskFile)
	if n := countRows(t, pool, `SELECT COUNT(*) FROM object_deletions WHERE bucket = 'logos'`); n != 0 {
		t.Error("an application delete must not queue the program logo")
	}

	if err := NewApplicationRepository(pool).Delete(ctx, f.ApplicationID); !errors.Is(err, domain.ErrApplicationNotFound) {
		t.Errorf("second delete err = %v; want ErrApplicationNotFound", err)
	}
}

func TestApplicationRepositoryIntegration_BulkDeclineByTermDeclinesOnlyPending(t *testing.T) {
	pool := integrationPool(t)
	f := seedCascadeFixture(t, pool) // the seeded application is accepted
	ctx := context.Background()
	pendingID, withdrawnID, otherTermID := "00000000-0000-0000-0000-000000000611", "00000000-0000-0000-0000-000000000612", "00000000-0000-0000-0000-000000000613"
	secondUser, thirdUser := "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003"
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ($1, 'second', 'Second'), ($2, 'third', 'Third')`, secondUser, thirdUser); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO applications (id, program_term_id, user_id, role, status, program_term_status) VALUES
		($1, $4, $6, 'mentee', 'pending', 'open'),
		($2, $4, $7, 'mentee', 'withdrawn', 'open'),
		($3, $5, $6, 'mentee', 'pending', 'closed')`, pendingID, withdrawnID, otherTermID, f.OpenTerm, f.ClosedTerm, secondUser, thirdUser); err != nil {
		t.Fatalf("insert applications: %v", err)
	}
	pendingTask := "00000000-0000-0000-0000-000000000614"
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status, application_status, program_term_status)
		VALUES ($1, $2, $3, $4, 'incomplete', 'pending', 'open')`, pendingTask, pendingID, f.OpenTerm, secondUser); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	declined, err := NewApplicationRepository(pool).BulkDeclineByTerm(ctx, f.OpenTerm)
	if err != nil {
		t.Fatalf("BulkDeclineByTerm: %v", err)
	}
	if len(declined) != 1 || declined[0].ID != pendingID || declined[0].Status != models.ApplicationStatusDeclined {
		t.Fatalf("declined = %+v; want only the pending application of this term", declined)
	}

	for id, want := range map[string]models.ApplicationStatus{
		pendingID:       models.ApplicationStatusDeclined,
		f.ApplicationID: models.ApplicationStatusAccepted,
		withdrawnID:     models.ApplicationStatusWithdrawn,
		otherTermID:     models.ApplicationStatusPending,
	} {
		var got models.ApplicationStatus
		if err := pool.QueryRow(ctx, `SELECT status FROM applications WHERE id = $1`, id).Scan(&got); err != nil || got != want {
			t.Errorf("application %s status = %q, err = %v; want %q", id, got, err, want)
		}
	}
	var taskAppStatus string
	if err := pool.QueryRow(ctx, `SELECT application_status FROM tasks WHERE id = $1`, pendingTask).Scan(&taskAppStatus); err != nil || taskAppStatus != "declined" {
		t.Errorf("task application_status = %q, err = %v; want declined", taskAppStatus, err)
	}
	assertFGAMarker(t, pool, "mentorship_application", pendingID, "update_access")
	if n := countRows(t, pool, `SELECT COUNT(*) FROM fga_outbox WHERE object_uid IN ($1, $2, $3)`, f.ApplicationID, withdrawnID, otherTermID); n != 0 {
		t.Errorf("untouched applications got %d FGA markers; want 0", n)
	}

	if again, err := NewApplicationRepository(pool).BulkDeclineByTerm(ctx, f.OpenTerm); err != nil || len(again) != 0 {
		t.Errorf("second run declined %d, err = %v; want nothing left to decline", len(again), err)
	}
}

func TestProgramTermRepositoryIntegration_CloseWithBulkDeclineGuards(t *testing.T) {
	pool := integrationPool(t)
	f := seedCascadeFixture(t, pool) // the seeded application is accepted
	ctx := context.Background()
	repo := NewProgramTermRepository(pool)

	if _, _, err := repo.CloseWithBulkDecline(ctx, f.OpenTerm); !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("close with an accepted application err = %v; want ErrStateLocked", err)
	}
	var status models.ProgramTermStatus
	if err := pool.QueryRow(ctx, `SELECT status FROM program_terms WHERE id = $1`, f.OpenTerm).Scan(&status); err != nil || status != models.ProgramTermStatusOpen {
		t.Fatalf("term status = %q, err = %v; want still open", status, err)
	}

	if _, _, err := repo.CloseWithBulkDecline(ctx, f.ClosedTerm); !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("close an already closed term err = %v; want ErrInvalidStateTransition", err)
	}
	if _, _, err := repo.CloseWithBulkDecline(ctx, "00000000-0000-0000-0000-000000000699"); !errors.Is(err, domain.ErrProgramTermNotFound) {
		t.Errorf("close a missing term err = %v; want ErrProgramTermNotFound", err)
	}

	// Once the accepted mentee has graduated, the term can close and pending applications are declined.
	if _, err := pool.Exec(ctx, `UPDATE applications SET status = 'graduated' WHERE id = $1`, f.ApplicationID); err != nil {
		t.Fatalf("graduate: %v", err)
	}
	pendingID, secondUser := "00000000-0000-0000-0000-000000000621", "00000000-0000-0000-0000-000000000002"
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ($1, 'second', 'Second')`, secondUser); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status, program_term_status) VALUES ($1, $2, $3, 'mentee', 'pending', 'open')`, pendingID, f.OpenTerm, secondUser); err != nil {
		t.Fatalf("insert pending application: %v", err)
	}
	term, declined, err := repo.CloseWithBulkDecline(ctx, f.OpenTerm)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if term.Status != models.ProgramTermStatusClosed || len(declined) != 1 || declined[0].ID != pendingID {
		t.Fatalf("closed term status %q, declined %+v; want closed with the pending application declined", term.Status, declined)
	}
	var graduatedTermStatus string
	if err := pool.QueryRow(ctx, `SELECT program_term_status FROM applications WHERE id = $1`, f.ApplicationID).Scan(&graduatedTermStatus); err != nil || graduatedTermStatus != "closed" {
		t.Errorf("graduated application program_term_status = %q, err = %v; want closed", graduatedTermStatus, err)
	}
	assertIndexAction(t, pool, "mentorship_program", f.ProgramID, "updated")
}
