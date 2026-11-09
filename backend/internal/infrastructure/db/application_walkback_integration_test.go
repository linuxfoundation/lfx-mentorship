// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// openExtraTerms adds n open terms to the fixture program and returns their IDs.
func openExtraTerms(t *testing.T, pool *pgxpool.Pool, programID string, n int) []string {
	t.Helper()
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("00000000-0000-0000-0004-%012d", i+1)
		if _, err := pool.Exec(context.Background(), `INSERT INTO program_terms (id, program_id, name, status) VALUES ($1, $2, $3, 'open')`,
			ids[i], programID, fmt.Sprintf("Extra %d", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func insertApplication(t *testing.T, pool *pgxpool.Pool, id, termID, userID, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO applications (id, program_term_id, user_id, role, status, attendance_type) VALUES ($1, $2, $3, 'mentee', $4, 'part_time')`,
		id, termID, userID, status); err != nil {
		t.Fatal(err)
	}
}

// reopen moves a mentee application back to pending as the service does, with the program's
// prerequisites as stored.
func reopen(t *testing.T, pool *pgxpool.Pool, id string, from models.ApplicationStatus, templates json.RawMessage, prerequisites []models.PrerequisiteTemplate) (*models.Application, error) {
	t.Helper()
	pending := from
	next := models.ApplicationStatusPending
	return NewApplicationRepository(pool).Update(context.Background(), id, models.ApplicationUpdateInput{
		Status: &next, ExpectedStatus: &pending,
		AddMissingPrerequisites: true, Prerequisites: prerequisites, ExpectedTaskTemplates: templates,
	})
}

func TestApplicationRepositoryIntegration_ReopenAddsMissingPrerequisites(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}

	withdrawn := s.apply(f.OpenTerm, "mentee", "withdrawn")
	s.task(withdrawn, "Resume", "prerequisite", "incomplete")
	s.task(withdrawn, "Essay", "prerequisite", "in_progress")
	s.task(withdrawn, "Video", "prerequisite", "submitted")
	s.task(withdrawn, "Intro call", "non_prerequisite", "incomplete")

	// While the application was withdrawn, the program edited Resume, dropped Essay and Video, and
	// added Cover letter, which a legacy import stored twice.
	templates := json.RawMessage(`[{"name": "resume", "description": "Upload a PDF"}, {"name": "Cover letter", "custom": true}, {"name": "cover letter"}]`)
	if _, err := pool.Exec(context.Background(), `UPDATE programs SET task_templates = $2 WHERE id = $1`, f.ProgramID, templates); err != nil {
		t.Fatal(err)
	}
	description := "Upload a PDF"
	a, err := reopen(t, pool, withdrawn, models.ApplicationStatusWithdrawn, templates, []models.PrerequisiteTemplate{
		{Name: "resume", Description: &description},
		{Name: "Cover letter", Custom: true},
		{Name: "cover letter"},
	})
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	if a.Status != models.ApplicationStatusPending {
		t.Fatalf("status = %s; want pending", a.Status)
	}

	// Existing tasks are kept as they are, since they may carry a reviewer's change for this
	// applicant; only the prerequisite with no task gets one, once.
	got := syncedTasks(t, pool, withdrawn)
	want := map[string]string{
		"Resume":       "incomplete",
		"Essay":        "in_progress",
		"Video":        "submitted",
		"Intro call":   "incomplete",
		"Cover letter": "incomplete",
	}
	if len(got) != len(want) {
		t.Fatalf("tasks = %v; want %v", keys(got), want)
	}
	for name, status := range want {
		if g, ok := got[name]; !ok || g.Status != status {
			t.Errorf("task %q = %+v; want status %s", name, g, status)
		}
	}
	if deref(got["Resume"].Description) != "Old description" {
		t.Errorf("Resume was changed: %+v", got["Resume"])
	}
	if !got["Cover letter"].Custom {
		t.Errorf("added task lost its custom flag: %+v", got["Cover letter"])
	}
	var stale int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM tasks WHERE application_id = $1 AND application_status <> 'pending'`, withdrawn).Scan(&stale); err != nil || stale != 0 {
		t.Fatalf("%d tasks kept the old application status (err %v)", stale, err)
	}
}

// A withdrawn application whose applicant has applied to the term again answers 409, even when the
// newer application has them at the pending limit.
func TestApplicationRepositoryIntegration_ReopenRefusedWhileReapplied(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	terms := openExtraTerms(t, pool, f.ProgramID, models.MaxPendingMenteeApplications-1)
	for i, term := range terms {
		insertApplication(t, pool, fmt.Sprintf("00000000-0000-0000-0007-%012d", i+1), term, f.UserID, "pending")
	}
	old := "00000000-0000-0000-0007-000000000098"
	insertApplication(t, pool, old, f.OpenTerm, f.UserID, "withdrawn")
	insertApplication(t, pool, "00000000-0000-0000-0007-000000000099", f.OpenTerm, f.UserID, "pending")
	if _, err := reopen(t, pool, old, models.ApplicationStatusWithdrawn, nil, nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v; want ErrConflict", err)
	}
}

func TestApplicationRepositoryIntegration_ReopenRefusedWhenTemplatesChanged(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	declined := (&syncSeeder{t: t, pool: pool}).apply(f.OpenTerm, "mentee", "declined")
	if _, err := pool.Exec(context.Background(), `UPDATE programs SET task_templates = '[{"name": "Essay"}]' WHERE id = $1`, f.ProgramID); err != nil {
		t.Fatal(err)
	}
	// The service read the templates before this edit.
	if _, err := reopen(t, pool, declined, models.ApplicationStatusDeclined, nil, nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v; want ErrConflict", err)
	}
}

func TestApplicationRepositoryIntegration_ReopenNeedsOpenTerm(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	withdrawn := (&syncSeeder{t: t, pool: pool}).apply(f.ClosedTerm, "mentee", "withdrawn")
	if _, err := reopen(t, pool, withdrawn, models.ApplicationStatusWithdrawn, nil, nil); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("err = %v; want ErrIneligible", err)
	}
}

func TestApplicationRepositoryIntegration_ReopenKeepsPendingLimit(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	terms := openExtraTerms(t, pool, f.ProgramID, models.MaxPendingMenteeApplications)
	for i, term := range terms {
		insertApplication(t, pool, fmt.Sprintf("00000000-0000-0000-0005-%012d", i+1), term, f.UserID, "pending")
	}
	declined := "00000000-0000-0000-0005-000000000099"
	insertApplication(t, pool, declined, f.OpenTerm, f.UserID, "declined")
	if _, err := reopen(t, pool, declined, models.ApplicationStatusDeclined, nil, nil); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("err = %v; want ErrIneligible", err)
	}

	// Withdrawing one frees a slot.
	if _, err := pool.Exec(context.Background(), `UPDATE applications SET status = 'withdrawn' WHERE id = '00000000-0000-0000-0005-000000000001'`); err != nil {
		t.Fatal(err)
	}
	if _, err := reopen(t, pool, declined, models.ApplicationStatusDeclined, nil, nil); err != nil {
		t.Fatalf("re-open under the limit: %v", err)
	}
}

// A second re-open of the same application, validated before the first committed, answers 409
// rather than counting the re-opened application against the pending limit.
func TestApplicationRepositoryIntegration_StaleReopenConflicts(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	terms := openExtraTerms(t, pool, f.ProgramID, models.MaxPendingMenteeApplications-1)
	for i, term := range terms {
		insertApplication(t, pool, fmt.Sprintf("00000000-0000-0000-0008-%012d", i+1), term, f.UserID, "pending")
	}
	declined := "00000000-0000-0000-0008-000000000099"
	insertApplication(t, pool, declined, f.OpenTerm, f.UserID, "declined")
	if _, err := reopen(t, pool, declined, models.ApplicationStatusDeclined, nil, nil); err != nil {
		t.Fatalf("first re-open: %v", err)
	}
	if _, err := reopen(t, pool, declined, models.ApplicationStatusDeclined, nil, nil); !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v; want ErrInvalidStateTransition", err)
	}
}

// Undoing a graduation goes through the accept path, so it keeps both of its guards.
func TestApplicationRepositoryIntegration_UndoGraduationGuards(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	repo := NewApplicationRepository(pool)
	graduated, accepted := models.ApplicationStatusGraduated, models.ApplicationStatusAccepted
	undo := func(id string) error {
		_, err := repo.Update(context.Background(), id, models.ApplicationUpdateInput{Status: &accepted, ExpectedStatus: &graduated})
		return err
	}

	t.Run("refused while the mentee holds another accepted mentorship", func(t *testing.T) {
		terms := openExtraTerms(t, pool, f.ProgramID, 1)
		app := "00000000-0000-0000-0006-000000000001"
		insertApplication(t, pool, app, f.OpenTerm, f.UserID, "graduated")
		insertApplication(t, pool, "00000000-0000-0000-0006-000000000002", terms[0], f.UserID, "accepted")
		if err := undo(app); !errors.Is(err, domain.ErrIneligible) {
			t.Fatalf("err = %v; want ErrIneligible", err)
		}
	})

	t.Run("refused once the term is closed", func(t *testing.T) {
		const user = "00000000-0000-0000-0006-0000000000aa"
		if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, lfid, name) VALUES ($1, 'closed-graduate', 'Graduate')`, user); err != nil {
			t.Fatal(err)
		}
		app := "00000000-0000-0000-0006-000000000003"
		insertApplication(t, pool, app, f.ClosedTerm, user, "graduated")
		if err := undo(app); !errors.Is(err, domain.ErrIneligible) {
			t.Fatalf("err = %v; want ErrIneligible", err)
		}
	})
}
