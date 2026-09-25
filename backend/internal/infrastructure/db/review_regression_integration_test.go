// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestIndexOutboxIntegration_StaleClaimDeadLettersAtLimit(t *testing.T) {
	pool := integrationPool(t)
	repo := NewIndexOutboxRepository(pool)
	repo.SetMaxAttempts(2)
	ctx := context.Background()

	if err := repo.Enqueue(ctx, indexOutboxFixtureRecord("mentorship_task", "00000000-0000-0000-0000-000000000301")); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 1 || claimed[0].Attempts != 1 {
		t.Fatalf("first claim = %#v, err=%v", claimed, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE index_outbox SET claimed_at = NOW() - INTERVAL '6 minutes'`); err != nil {
		t.Fatalf("age first claim: %v", err)
	}
	claimed, err = repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 1 || claimed[0].Attempts != 2 {
		t.Fatalf("second claim = %#v, err=%v", claimed, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE index_outbox SET claimed_at = NOW() - INTERVAL '6 minutes'`); err != nil {
		t.Fatalf("age second claim: %v", err)
	}
	claimed, err = repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("exhausted claim = %#v, err=%v", claimed, err)
	}
	var state, lastError string
	if err := pool.QueryRow(ctx, `SELECT state, last_error FROM index_outbox`).Scan(&state, &lastError); err != nil {
		t.Fatalf("read dead-letter record: %v", err)
	}
	if state != "dead_letter" || lastError == "" {
		t.Fatalf("dead-letter state = %q, error = %q", state, lastError)
	}
}

func TestProgramTermIntegration_CloseReopenSynchronizesProjections(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	applicationID := "00000000-0000-0000-0000-000000000401"
	taskID := "00000000-0000-0000-0000-000000000402"
	if _, err := pool.Exec(ctx, `
		INSERT INTO applications (id, program_term_id, user_id, role, status, program_term_status)
		VALUES ($1, $2, $3, 'mentee', 'pending', 'open')`, applicationID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status, application_status, program_term_status)
		VALUES ($1, $2, $3, $4, 'incomplete', 'pending', 'open')`, taskID, applicationID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	repo := NewProgramTermRepository(pool)
	if _, declined, err := repo.CloseWithBulkDecline(ctx, fixture.OpenTerm); err != nil || declined != 1 {
		t.Fatalf("close term declined=%d, err=%v", declined, err)
	}
	assertTermProjectionStatus(t, pool, fixture.OpenTerm, "closed")

	open := models.ProgramTermStatusOpen
	if _, err := repo.Update(ctx, fixture.OpenTerm, models.ProgramTermUpdateInput{Status: &open}); err != nil {
		t.Fatalf("reopen term: %v", err)
	}
	assertTermProjectionStatus(t, pool, fixture.OpenTerm, "open")
}

func assertTermProjectionStatus(t *testing.T, pool *pgxpool.Pool, termID, want string) {
	t.Helper()
	var applicationStatus, taskStatus, indexedStatus string
	if err := pool.QueryRow(context.Background(), `SELECT program_term_status FROM applications WHERE program_term_id = $1`, termID).Scan(&applicationStatus); err != nil {
		t.Fatalf("read application projection: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT program_term_status FROM tasks WHERE program_term_id = $1`, termID).Scan(&taskStatus); err != nil {
		t.Fatalf("read task projection: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT data->>'program_term_status' FROM index_outbox WHERE object_type = 'mentorship_task' LIMIT 1`).Scan(&indexedStatus); err != nil {
		t.Fatalf("read task index projection: %v", err)
	}
	if applicationStatus != want || taskStatus != want || indexedStatus != want {
		t.Fatalf("projection statuses = application=%q task=%q index=%q, want %q", applicationStatus, taskStatus, indexedStatus, want)
	}
}

func indexOutboxFixtureRecord(objectType, objectUID string) domain.IndexOutboxRecord {
	return domain.IndexOutboxRecord{ObjectType: objectType, ObjectUID: objectUID, Action: "updated", Headers: []byte(`{}`), Data: []byte(`{}`), IndexingConfig: []byte(`{}`)}
}
