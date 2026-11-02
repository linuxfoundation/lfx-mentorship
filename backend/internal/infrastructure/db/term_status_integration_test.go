// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// insertTermEnding inserts a term of the program with the given status that ends at end.
func insertTermEnding(t *testing.T, pool *pgxpool.Pool, id, programID, status string, end time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO program_terms (id, program_id, name, status, start_date_time, end_date_time) VALUES ($1, $2, $3, $4, $5, $6)`,
		id, programID, "Term "+id, status, end.AddDate(0, -3, 0), end); err != nil {
		t.Fatalf("insert term %s: %v", id, err)
	}
}

// insertMenteeApplication inserts a mentee application of the user on an open term.
func insertMenteeApplication(t *testing.T, pool *pgxpool.Pool, id, termID, userID, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO applications (id, program_term_id, user_id, role, status, program_term_status) VALUES ($1, $2, $3, 'mentee', $4, 'open')`,
		id, termID, userID, status); err != nil {
		t.Fatalf("insert application %s: %v", id, err)
	}
}

func TestProgramTermRepositoryIntegration_CloseEnded(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool) // its open term has no end date, so it never counts as ended
	ctx := context.Background()
	repo := NewProgramTermRepository(pool)
	now := time.Date(2026, 11, 2, 0, 10, 0, 0, time.UTC)

	const (
		ended          = "00000000-0000-0000-0000-000000000701"
		endedGraduated = "00000000-0000-0000-0000-000000000702"
		pendingMentee  = "00000000-0000-0000-0000-000000000703"
		acceptedMentee = "00000000-0000-0000-0000-000000000704"
		notEnded       = "00000000-0000-0000-0000-000000000705"
		alreadyClosed  = "00000000-0000-0000-0000-000000000706"
		graduatedApp   = "00000000-0000-0000-0000-000000000712"
	)
	insertTermEnding(t, pool, ended, f.ProgramID, "open", now.AddDate(0, 0, -1))
	insertTermEnding(t, pool, endedGraduated, f.ProgramID, "open", now.AddDate(-1, 0, 0))
	insertTermEnding(t, pool, pendingMentee, f.ProgramID, "open", now.AddDate(0, 0, -1))
	insertTermEnding(t, pool, acceptedMentee, f.ProgramID, "open", now.AddDate(0, 0, -1))
	insertTermEnding(t, pool, notEnded, f.ProgramID, "open", now.Add(time.Minute))
	insertTermEnding(t, pool, alreadyClosed, f.ProgramID, "closed", now.AddDate(0, 0, -1))
	// A graduated mentee is done and does not hold a term open; pending and accepted ones do.
	insertMenteeApplication(t, pool, graduatedApp, endedGraduated, f.UserID, "graduated")
	insertMenteeApplication(t, pool, "00000000-0000-0000-0000-000000000713", pendingMentee, f.UserID, "pending")
	insertMenteeApplication(t, pool, "00000000-0000-0000-0000-000000000714", acceptedMentee, f.UserID, "accepted")
	if _, err := pool.Exec(ctx, `TRUNCATE fga_outbox, index_outbox`); err != nil {
		t.Fatalf("clear outboxes: %v", err)
	}

	ids, err := repo.ListEndedIDs(ctx, now)
	if err != nil {
		t.Fatalf("ListEndedIDs: %v", err)
	}
	if want := []string{endedGraduated, ended}; !slices.Equal(ids, want) {
		t.Fatalf("ListEndedIDs = %v; want %v, oldest end first", ids, want)
	}

	for _, id := range ids {
		term, err := repo.CloseEnded(ctx, id, now)
		if err != nil {
			t.Fatalf("CloseEnded(%s): %v", id, err)
		}
		if term.Status != models.ProgramTermStatusClosed {
			t.Errorf("term %s status = %q; want closed", id, term.Status)
		}
	}
	var termStatus string
	if err := pool.QueryRow(ctx, `SELECT program_term_status FROM applications WHERE id = $1`, graduatedApp).Scan(&termStatus); err != nil || termStatus != "closed" {
		t.Errorf("graduated application program_term_status = %q, err = %v; want closed", termStatus, err)
	}
	assertFGAMarker(t, pool, "mentorship_application", graduatedApp, "update_access")
	assertIndexAction(t, pool, "mentorship_program", f.ProgramID, "updated")

	for _, id := range []string{ended, pendingMentee, acceptedMentee, notEnded, alreadyClosed, f.OpenTerm} {
		if _, err := repo.CloseEnded(ctx, id, now); !errors.Is(err, domain.ErrInvalidStateTransition) {
			t.Errorf("CloseEnded(%s) err = %v; want ErrInvalidStateTransition", id, err)
		}
	}
	if _, err := repo.CloseEnded(ctx, "00000000-0000-0000-0000-000000000799", now); !errors.Is(err, domain.ErrProgramTermNotFound) {
		t.Errorf("CloseEnded(missing) err = %v; want ErrProgramTermNotFound", err)
	}
	if ids, err := repo.ListEndedIDs(ctx, now); err != nil || len(ids) != 0 {
		t.Errorf("second ListEndedIDs = %v, err = %v; want none left", ids, err)
	}
}

// A reopen locks only its application, not the term. CloseEnded must wait for one in flight and see
// the reopened pending mentee, rather than close the term on it.
func TestProgramTermRepositoryIntegration_CloseEndedWaitsForReopen(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	now := time.Date(2026, 11, 2, 0, 10, 0, 0, time.UTC)
	const termID, applicationID = "00000000-0000-0000-0000-000000000721", "00000000-0000-0000-0000-000000000722"
	insertTermEnding(t, pool, termID, f.ProgramID, "open", now.AddDate(0, 0, -1))
	insertMenteeApplication(t, pool, applicationID, termID, f.UserID, "declined")

	reopen, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin reopen: %v", err)
	}
	defer func() { _ = reopen.Rollback(ctx) }()
	if _, err := reopen.Exec(ctx, `UPDATE applications SET status = 'pending' WHERE id = $1`, applicationID); err != nil {
		t.Fatalf("reopen application: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := NewProgramTermRepository(pool).CloseEnded(ctx, termID, now)
		done <- err
	}()
	waitForLockWait(t, pool, done)
	if err := reopen.Commit(ctx); err != nil {
		t.Fatalf("commit reopen: %v", err)
	}
	if err := <-done; !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("CloseEnded err = %v; want ErrInvalidStateTransition once the reopen commits", err)
	}
	var status models.ProgramTermStatus
	if err := pool.QueryRow(ctx, `SELECT status FROM program_terms WHERE id = $1`, termID).Scan(&status); err != nil || status != models.ProgramTermStatusOpen {
		t.Errorf("term status = %q, err = %v; want still open", status, err)
	}
}
