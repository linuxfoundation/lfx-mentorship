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

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestProgramTermRepositoryIntegration_CloseEnded(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool) // its open term has no end date, so it never counts as ended
	ctx := context.Background()
	repo := NewProgramTermRepository(pool)
	now := time.Date(2026, 11, 2, 0, 10, 0, 0, time.UTC)

	const (
		ended           = "00000000-0000-0000-0000-000000000701"
		endedWithMentor = "00000000-0000-0000-0000-000000000702"
		pendingMentee   = "00000000-0000-0000-0000-000000000703"
		acceptedMentee  = "00000000-0000-0000-0000-000000000704"
		notEnded        = "00000000-0000-0000-0000-000000000705"
		alreadyClosed   = "00000000-0000-0000-0000-000000000706"
	)
	terms := []struct {
		id, status string
		end        time.Time
	}{
		{ended, "open", now.AddDate(0, 0, -1)},
		{endedWithMentor, "open", now.AddDate(-1, 0, 0)},
		{pendingMentee, "open", now.AddDate(0, 0, -1)},
		{acceptedMentee, "open", now.AddDate(0, 0, -1)},
		{notEnded, "open", now.Add(time.Minute)},
		{alreadyClosed, "closed", now.AddDate(0, 0, -1)},
	}
	for _, term := range terms {
		if _, err := pool.Exec(ctx, `INSERT INTO program_terms (id, program_id, name, status, start_date_time, end_date_time) VALUES ($1, $2, $1, $3, $4, $5)`,
			term.id, f.ProgramID, term.status, term.end.AddDate(0, -3, 0), term.end); err != nil {
			t.Fatalf("insert term %s: %v", term.id, err)
		}
	}
	applications := []struct {
		id, term, role, status string
	}{
		// An accepted mentor stays accepted for good, and a graduated mentee is done; neither holds a term open.
		{"00000000-0000-0000-0000-000000000711", endedWithMentor, "mentor", "accepted"},
		{"00000000-0000-0000-0000-000000000712", endedWithMentor, "mentee", "graduated"},
		{"00000000-0000-0000-0000-000000000713", pendingMentee, "mentee", "pending"},
		{"00000000-0000-0000-0000-000000000714", acceptedMentee, "mentee", "accepted"},
	}
	for _, a := range applications {
		if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status, program_term_status) VALUES ($1, $2, $3, $4, $5, 'open')`,
			a.id, a.term, f.UserID, a.role, a.status); err != nil {
			t.Fatalf("insert application %s: %v", a.id, err)
		}
	}
	if _, err := pool.Exec(ctx, `TRUNCATE fga_outbox, index_outbox`); err != nil {
		t.Fatalf("clear outboxes: %v", err)
	}

	ids, err := repo.ListEndedIDs(ctx, now)
	if err != nil {
		t.Fatalf("ListEndedIDs: %v", err)
	}
	if want := []string{endedWithMentor, ended}; !slices.Equal(ids, want) {
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
	for _, a := range applications[:2] {
		var termStatus string
		if err := pool.QueryRow(ctx, `SELECT program_term_status FROM applications WHERE id = $1`, a.id).Scan(&termStatus); err != nil || termStatus != "closed" {
			t.Errorf("application %s program_term_status = %q, err = %v; want closed", a.id, termStatus, err)
		}
		assertFGAMarker(t, pool, "mentorship_application", a.id, "update_access")
	}
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
