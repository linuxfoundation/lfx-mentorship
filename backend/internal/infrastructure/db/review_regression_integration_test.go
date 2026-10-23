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

	"github.com/jackc/pgx/v5/pgconn"
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
	start := indexOutboxStaleDeadLettered.Value()
	claimed, err = repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("exhausted claim = %#v, err=%v", claimed, err)
	}
	if got := indexOutboxStaleDeadLettered.Value() - start; got != 1 {
		t.Fatalf("stale dead-letter count delta=%d, want 1", got)
	}
	var state, lastError string
	if err := pool.QueryRow(ctx, `SELECT state, last_error FROM index_outbox`).Scan(&state, &lastError); err != nil {
		t.Fatalf("read dead-letter record: %v", err)
	}
	if state != "dead_letter" || lastError == "" {
		t.Fatalf("dead-letter state = %q, error = %q", state, lastError)
	}
}

func TestFGAOutboxIntegration_StaleSameGenerationDeadLettersAtLimit(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	repo.SetMaxAttempts(1)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_task", "task-stale", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if markers, err := repo.Claim(ctx, 1); err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fga_outbox SET claimed_at = NOW() - INTERVAL '6 minutes'`); err != nil {
		t.Fatalf("age claim: %v", err)
	}
	start := fgaOutboxStaleDeadLettered.Value()
	if markers, err := repo.Claim(ctx, 1); err != nil || len(markers) != 0 {
		t.Fatalf("exhausted claim: markers=%d err=%v", len(markers), err)
	}
	if got := fgaOutboxStaleDeadLettered.Value() - start; got != 1 {
		t.Fatalf("stale dead-letter count delta=%d, want 1", got)
	}
	var state, lastError string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT state, attempts, last_error FROM fga_outbox`).Scan(&state, &attempts, &lastError); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if state != "dead_letter" || attempts != 1 || lastError == "" {
		t.Fatalf("state=%q attempts=%d last_error=%q; want dead_letter, 1, and an error", state, attempts, lastError)
	}
}

func TestFGAOutboxIntegration_StaleNewerGenerationIsReclaimedAtLimit(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	repo.SetMaxAttempts(1)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_task", "task-newer", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	first, err := repo.Claim(ctx, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(first), err)
	}
	if err := repo.EnqueueObject(ctx, "mentorship_task", "task-newer", "update_access"); err != nil {
		t.Fatalf("enqueue newer generation: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fga_outbox SET claimed_at = NOW() - INTERVAL '6 minutes'`); err != nil {
		t.Fatalf("age claim: %v", err)
	}
	reclaimed, err := repo.Claim(ctx, 1)
	if err != nil || len(reclaimed) != 1 {
		t.Fatalf("reclaim newer generation: markers=%d err=%v", len(reclaimed), err)
	}
	if reclaimed[0].Generation <= first[0].Generation || reclaimed[0].Attempts != 0 {
		t.Fatalf("reclaimed generation=%d attempts=%d; want generation > %d and attempts 0", reclaimed[0].Generation, reclaimed[0].Attempts, first[0].Generation)
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
	if _, declined, err := repo.CloseWithBulkDecline(ctx, fixture.OpenTerm); err != nil || len(declined) != 1 || declined[0].ID != applicationID {
		t.Fatalf("close term declined=%+v, err=%v", declined, err)
	}
	assertTermProjectionStatus(t, pool, fixture.OpenTerm, "closed")

	open := models.ProgramTermStatusOpen
	if _, err := repo.Update(ctx, fixture.OpenTerm, models.ProgramTermUpdateInput{Status: &open}); err != nil {
		t.Fatalf("reopen term: %v", err)
	}
	assertTermProjectionStatus(t, pool, fixture.OpenTerm, "open")
}

func TestProgramIntegration_EnrollmentTemplateReadsFundingStats(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO program_funding_stats (program_id, amount_raised) VALUES ($1, 1234.5)`, fixture.ProgramID); err != nil {
		t.Fatalf("insert funding stats: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_skills (program_id, skill) VALUES ($1, 'Go')`, fixture.ProgramID); err != nil {
		t.Fatalf("insert skill: %v", err)
	}

	template, err := NewProgramRepository(pool).GetEnrollmentTemplate(ctx, fixture.ProgramID)
	if err != nil {
		t.Fatalf("GetEnrollmentTemplate: %v", err)
	}
	if template.Program.AmountRaised != 1234.5 {
		t.Errorf("amount_raised = %v, want 1234.5 from program_funding_stats", template.Program.AmountRaised)
	}
	if len(template.Skills) != 1 || template.Skills[0] != "Go" {
		t.Errorf("skills = %v, want [Go]", template.Skills)
	}
}

func TestProgramIntegration_FundingTotalsCountOnlyPublishedPrograms(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	hiddenID := "00000000-0000-0000-0000-000000000501"
	if _, err := pool.Exec(ctx, `INSERT INTO programs (id, lf_project_uid, name, slug, status) VALUES ($1, '00000000-0000-0000-0000-000000000099', 'Hidden Program', 'hidden-program', 'hidden')`, hiddenID); err != nil {
		t.Fatalf("insert hidden program: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_funding_stats (program_id, amount_raised, amount_spent) VALUES ($1, 100, 40), ($2, 9000, 7000)`, fixture.ProgramID, hiddenID); err != nil {
		t.Fatalf("insert funding stats: %v", err)
	}

	raised, spent, err := NewProgramRepository(pool).GetFundingTotals(ctx)
	if err != nil {
		t.Fatalf("GetFundingTotals: %v", err)
	}
	if raised != 100 || spent != 40 {
		t.Errorf("totals = raised %v, spent %v; want 100, 40 (the hidden program must not be counted)", raised, spent)
	}
}

func TestApplicationRepositoryIntegration_ListByUserReturnsProjectName(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	projectName, logoURL := "Fixture Project", "https://example.com/program.svg"
	if _, err := pool.Exec(ctx, `UPDATE programs SET lf_project_name = $2, logo_url = $3 WHERE id = $1`, fixture.ProgramID, projectName, logoURL); err != nil {
		t.Fatalf("set program project metadata: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000070', $1, $2, 'mentee', 'pending')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}

	apps, _, err := NewApplicationRepository(pool).ListByUser(ctx, fixture.UserID, models.ApplicationFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(apps) != 1 || apps[0].Program == nil || apps[0].Term == nil {
		t.Fatalf("apps = %+v, want one application with program and term", apps)
	}
	program := apps[0].Program
	if program.ProjectName == nil || *program.ProjectName != projectName {
		t.Errorf("program.project_name = %v, want %q", program.ProjectName, projectName)
	}
	if program.LogoURL == nil || *program.LogoURL != logoURL {
		t.Errorf("program.logo_url = %v, want %q", program.LogoURL, logoURL)
	}
	if apps[0].Term.ID != fixture.OpenTerm {
		t.Errorf("term.id = %q, want %q", apps[0].Term.ID, fixture.OpenTerm)
	}
}

func TestApplicationRepositoryIntegration_MarkTasksSubmittedFlipsOnceWhenPrerequisitesDone(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	applicationID := "00000000-0000-0000-0000-000000000071"
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'pending')`, applicationID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}
	repo := NewApplicationRepository(pool)
	mark := func(step string, want bool) {
		t.Helper()
		if flipped, err := repo.MarkTasksSubmitted(ctx, applicationID); err != nil || flipped != want {
			t.Fatalf("%s: flipped = %v, err = %v; want %v", step, flipped, err, want)
		}
	}

	mark("no prerequisites", false)
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status, category, application_status, program_term_status) VALUES
		('00000000-0000-0000-0000-000000000073', $1, $2, $3, 'submitted', 'prerequisite', 'pending', 'open'),
		('00000000-0000-0000-0000-000000000074', $1, $2, $3, 'incomplete', 'prerequisite', 'pending', 'open'),
		('00000000-0000-0000-0000-000000000075', $1, $2, $3, 'incomplete', 'non_prerequisite', 'pending', 'open')`,
		applicationID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert tasks: %v", err)
	}
	mark("a prerequisite still incomplete", false)
	if _, err := pool.Exec(ctx, `UPDATE tasks SET status = 'complete' WHERE id = '00000000-0000-0000-0000-000000000074'`); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	mark("every prerequisite done", true)
	mark("already flagged", false)

	var indexed bool
	if err := pool.QueryRow(ctx, `SELECT (data->>'tasks_submitted')::boolean FROM index_outbox WHERE object_type = 'mentorship_application' AND object_uid = $1`, applicationID).Scan(&indexed); err != nil || !indexed {
		t.Fatalf("indexed tasks_submitted = %v, err = %v; want one refreshed document", indexed, err)
	}
	if flipped, err := repo.MarkTasksSubmitted(ctx, "00000000-0000-0000-0000-000000000072"); err != nil || flipped {
		t.Fatalf("missing application: flipped = %v, err = %v; want false, nil", flipped, err)
	}
}

func TestApplicationRepositoryIntegration_StatusChangeAppliesOnlyFromExpectedStatus(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	applicationID := "00000000-0000-0000-0000-000000000076"
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'pending')`, applicationID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}
	repo := NewApplicationRepository(pool)
	pending, accepted, declined := models.ApplicationStatusPending, models.ApplicationStatusAccepted, models.ApplicationStatusDeclined

	// Two reviewers both validated against pending; the decline commits first.
	if _, err := repo.Update(ctx, applicationID, models.ApplicationUpdateInput{Status: &declined, ExpectedStatus: &pending}); err != nil {
		t.Fatalf("decline: %v", err)
	}
	if _, err := repo.Update(ctx, applicationID, models.ApplicationUpdateInput{Status: &accepted, ExpectedStatus: &pending}); !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("stale accept err = %v; want ErrInvalidStateTransition", err)
	}
	var status models.ApplicationStatus
	if err := pool.QueryRow(ctx, `SELECT status FROM applications WHERE id = $1`, applicationID).Scan(&status); err != nil || status != declined {
		t.Fatalf("status = %q, err = %v; want declined", status, err)
	}
}

func TestProgramRepositoryIntegration_StatusChangeAppliesOnlyFromExpectedStatus(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	repo := NewProgramRepository(pool)
	published, hidden, archived := models.ProgramStatusPublished, models.ProgramStatusHidden, models.ProgramStatusArchived
	if _, _, err := repo.Update(ctx, fixture.ProgramID, models.ProgramUpdateInput{Status: &hidden, ExpectedStatus: &published}, models.PrerequisiteChanges{}); err != nil {
		t.Fatalf("hide: %v", err)
	}

	// An unhide validated against hidden must not overwrite an archive that committed first.
	if _, _, err := repo.Update(ctx, fixture.ProgramID, models.ProgramUpdateInput{Status: &archived, ExpectedStatus: &hidden}, models.PrerequisiteChanges{}); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, _, err := repo.Update(ctx, fixture.ProgramID, models.ProgramUpdateInput{Status: &published, ExpectedStatus: &hidden}, models.PrerequisiteChanges{}); !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("stale unhide err = %v; want ErrInvalidStateTransition", err)
	}
	var status models.ProgramStatus
	if err := pool.QueryRow(ctx, `SELECT status FROM programs WHERE id = $1`, fixture.ProgramID).Scan(&status); err != nil || status != archived {
		t.Fatalf("status = %q, err = %v; want archived", status, err)
	}
}

func TestProgramRepositoryIntegration_HideWaitsForInFlightApplication(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})

	// An application create that passed its checks holds the term lock and has not committed.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM program_terms WHERE id = $1 FOR UPDATE`, fixture.OpenTerm); err != nil {
		t.Fatalf("lock term: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000077', $1, $2, 'mentee', 'pending')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}

	published, hidden := models.ProgramStatusPublished, models.ProgramStatusHidden
	hideErr := make(chan error, 1)
	go func() {
		_, _, err := NewProgramRepository(pool).Update(ctx, fixture.ProgramID, models.ProgramUpdateInput{Status: &hidden, ExpectedStatus: &published}, models.PrerequisiteChanges{})
		hideErr <- err
	}()
	waitForLockWait(t, pool, hideErr)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit application: %v", err)
	}
	if err := <-hideErr; !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("hide err = %v; want ErrStateLocked", err)
	}
	var status models.ProgramStatus
	if err := pool.QueryRow(ctx, `SELECT status FROM programs WHERE id = $1`, fixture.ProgramID).Scan(&status); err != nil || status != published {
		t.Fatalf("status = %q, err = %v; want published", status, err)
	}
}

func TestProgramRepositoryIntegration_HideWaitsForApplicationStatusChange(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000080', $1, $2, 'mentee', 'declined')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}

	// A declined → pending reopen that passed its checks holds the application row and has not committed.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE applications SET status = 'pending' WHERE id = '00000000-0000-0000-0000-000000000080'`); err != nil {
		t.Fatalf("reopen application: %v", err)
	}

	published, hidden := models.ProgramStatusPublished, models.ProgramStatusHidden
	hideErr := make(chan error, 1)
	go func() {
		_, _, err := NewProgramRepository(pool).Update(ctx, fixture.ProgramID, models.ProgramUpdateInput{Status: &hidden, ExpectedStatus: &published}, models.PrerequisiteChanges{})
		hideErr <- err
	}()
	waitForLockWait(t, pool, hideErr)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit reopen: %v", err)
	}
	if err := <-hideErr; !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("hide err = %v; want ErrStateLocked", err)
	}
}

func TestApplicationRepositoryIntegration_StatusChangeWaitsForHide(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	applicationID := "00000000-0000-0000-0000-000000000081"
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'declined')`, applicationID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}

	// A hide holds its locks and has not committed.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM programs WHERE id = $1 FOR UPDATE`, fixture.ProgramID); err != nil {
		t.Fatalf("lock program: %v", err)
	}
	if err := checkNoBlockingApplications(ctx, tx, fixture.ProgramID); err != nil {
		t.Fatalf("hide guard: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE programs SET status = 'hidden' WHERE id = $1`, fixture.ProgramID); err != nil {
		t.Fatalf("hide: %v", err)
	}

	repo := NewApplicationRepository(pool)
	declined, pending := models.ApplicationStatusDeclined, models.ApplicationStatusPending
	reopenErr := make(chan error, 1)
	go func() {
		_, err := repo.Update(ctx, applicationID, models.ApplicationUpdateInput{Status: &pending, ExpectedStatus: &declined})
		reopenErr <- err
	}()
	waitForLockWait(t, pool, reopenErr)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit hide: %v", err)
	}
	if err := <-reopenErr; !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("reopen err = %v; want ErrIneligible", err)
	}
	var status models.ApplicationStatus
	if err := pool.QueryRow(ctx, `SELECT status FROM applications WHERE id = $1`, applicationID).Scan(&status); err != nil || status != declined {
		t.Fatalf("status = %q, err = %v; want declined", status, err)
	}
}

func TestApplicationRepositoryIntegration_RefusesApplicationToUnpublishedProgram(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	withdrawnID := "00000000-0000-0000-0000-000000000078"
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'withdrawn')`, withdrawnID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert withdrawn application: %v", err)
	}
	published, hidden := models.ProgramStatusPublished, models.ProgramStatusHidden
	if _, _, err := NewProgramRepository(pool).Update(ctx, fixture.ProgramID, models.ProgramUpdateInput{Status: &hidden, ExpectedStatus: &published}, models.PrerequisiteChanges{}); err != nil {
		t.Fatalf("hide: %v", err)
	}

	repo := NewApplicationRepository(pool)
	input := models.ApplicationCreateInput{UserID: fixture.UserID, Role: models.ApplicationRoleMentee, Status: models.ApplicationStatusPending}
	input.ID = "00000000-0000-0000-0000-000000000079"
	if _, err := repo.Create(ctx, fixture.OpenTerm, input); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("create err = %v; want ErrIneligible", err)
	}
	if _, err := repo.Reapply(ctx, withdrawnID, fixture.OpenTerm, input); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("reapply err = %v; want ErrIneligible", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE programs SET status = 'archived' WHERE id = $1`, fixture.ProgramID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := repo.Create(ctx, fixture.OpenTerm, input); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("create on archived program err = %v; want ErrIneligible", err)
	}
}

func TestApplicationRepositoryIntegration_ReapplyKeepsWithdrawnApplication(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	oldID, newID := "00000000-0000-0000-0000-000000000077", "00000000-0000-0000-0000-00000000007b"
	oldTaskID := "00000000-0000-0000-0000-000000000078"
	// The withdrawn application's created_on is later than the reapplication's will be, so the live
	// application must win on status rather than timestamp. The user also holds a live mentor application.
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status, reviewer_note, evaluation, created_on) VALUES ($1, $2, $3, 'mentee', 'withdrawn', 'do not accept', 'partial', NOW() + INTERVAL '1 day')`, oldID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000079', $1, $2, 'mentor', 'pending')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert mentor application: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status, category, application_status, program_term_status) VALUES ($1, $2, $3, $4, 'incomplete', 'non_prerequisite', 'withdrawn', 'open')`, oldTaskID, oldID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	repo := NewApplicationRepository(pool)
	input := models.ApplicationCreateInput{ID: newID, UserID: fixture.UserID, Role: models.ApplicationRoleMentee, Status: models.ApplicationStatusPending}

	a, err := repo.ReapplyWithTasks(ctx, oldID, fixture.OpenTerm, input, nil)
	if err != nil {
		t.Fatalf("ReapplyWithTasks: %v", err)
	}
	if a.ID != newID || a.Status != models.ApplicationStatusPending {
		t.Fatalf("reapplication = id %q status %q; want a new pending application %q", a.ID, a.Status, newID)
	}

	// The withdrawn application, its reviewer data, and its tasks are kept untouched.
	old, err := repo.GetByID(ctx, oldID)
	if err != nil {
		t.Fatalf("withdrawn application after reapply: %v", err)
	}
	if old.Status != models.ApplicationStatusWithdrawn || old.ReviewerNote == nil || *old.ReviewerNote != "do not accept" || old.Evaluation == nil || *old.Evaluation != "partial" {
		t.Fatalf("withdrawn application = status %q note %v evaluation %v; want it unchanged", old.Status, old.ReviewerNote, old.Evaluation)
	}
	var taskParent string
	if err := pool.QueryRow(ctx, `SELECT application_id FROM tasks WHERE id = $1`, oldTaskID).Scan(&taskParent); err != nil || taskParent != oldID {
		t.Fatalf("withdrawn application task parent = %q, err = %v; want %q", taskParent, err, oldID)
	}

	// The reapply guard now sees the live mentee application, not the withdrawn one or the mentor one.
	current, err := repo.FindByTermAndUser(ctx, fixture.OpenTerm, fixture.UserID, models.ApplicationRoleMentee)
	if err != nil || current == nil || current.ID != newID {
		t.Fatalf("FindByTermAndUser = %+v, err = %v; want %q", current, err, newID)
	}

	// Only the withdrawn application counts toward the reapplication limit.
	if n, err := repo.CountWithdrawnByTermAndUser(ctx, fixture.OpenTerm, fixture.UserID, models.ApplicationRoleMentee); err != nil || n != 1 {
		t.Fatalf("CountWithdrawnByTermAndUser = %d, err = %v; want 1", n, err)
	}

	// The applicants list shows the reapplication only, and agrees with the summary count.
	rows, meta, err := repo.ListByProgram(ctx, fixture.ProgramID, models.ProgramApplicationFilter{})
	if err != nil || meta.Total != 1 || len(rows) != 1 || rows[0].ApplicationID != newID {
		t.Fatalf("ListByProgram = %d rows, meta %+v, err = %v; want only %q", len(rows), meta, err, newID)
	}
	summary, err := NewProgramRepository(pool).GetManagementSummary(ctx, fixture.ProgramID)
	if err != nil || summary.Applicants != 1 {
		t.Fatalf("GetManagementSummary = %+v, err = %v; want 1 applicant", summary, err)
	}

	// A second reapply from the same withdrawn application cannot add another live one.
	input.ID = "00000000-0000-0000-0000-00000000007c"
	var pgErr *pgconn.PgError
	if _, err := repo.ReapplyWithTasks(ctx, oldID, fixture.OpenTerm, input, nil); !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("second reapply err = %v; want unique violation", err)
	}
	if _, err := repo.ReapplyWithTasks(ctx, newID, fixture.OpenTerm, input, nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("reapply from pending err = %v; want ErrConflict", err)
	}
}

func TestApplicationRepositoryIntegration_ReapplyEnforcesWithdrawnLimit(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	withdrawnIDs := []string{"00000000-0000-0000-0000-000000000081", "00000000-0000-0000-0000-000000000082", "00000000-0000-0000-0000-000000000083"}
	for _, id := range withdrawnIDs {
		// Identical created_on, so the list must break the tie on id.
		if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status, created_on) VALUES ($1, $2, $3, 'mentee', 'withdrawn', '2026-01-01T00:00:00Z')`, id, fixture.OpenTerm, fixture.UserID); err != nil {
			t.Fatalf("insert withdrawn application: %v", err)
		}
	}
	repo := NewApplicationRepository(pool)
	input := models.ApplicationCreateInput{ID: "00000000-0000-0000-0000-000000000084", UserID: fixture.UserID, Role: models.ApplicationRoleMentee, Status: models.ApplicationStatusPending}
	if _, err := repo.ReapplyWithTasks(ctx, withdrawnIDs[2], fixture.OpenTerm, input, nil); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("reapply past the limit err = %v; want ErrIneligible", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM applications WHERE id = $1`, input.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("applications created past the limit = %d, err = %v; want 0", count, err)
	}

	// Only the newest withdrawn application is listed, one applicant as in the summary.
	rows, meta, err := repo.ListByProgram(ctx, fixture.ProgramID, models.ProgramApplicationFilter{})
	if err != nil || meta.Total != 1 || len(rows) != 1 || rows[0].ApplicationID != withdrawnIDs[2] {
		t.Fatalf("ListByProgram = %d rows, meta %+v, err = %v; want only %q", len(rows), meta, err, withdrawnIDs[2])
	}
}

func TestApplicationRepositoryIntegration_ReapplyRejectsClosedTerm(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	oldID := "00000000-0000-0000-0000-000000000085"
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'withdrawn')`, oldID, fixture.ClosedTerm, fixture.UserID); err != nil {
		t.Fatalf("insert withdrawn application: %v", err)
	}
	input := models.ApplicationCreateInput{ID: "00000000-0000-0000-0000-000000000086", UserID: fixture.UserID, Role: models.ApplicationRoleMentee, Status: models.ApplicationStatusPending}
	if _, err := NewApplicationRepository(pool).ReapplyWithTasks(ctx, oldID, fixture.ClosedTerm, input, nil); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("reapply to a closed term err = %v; want ErrIneligible", err)
	}
}

// A withdrawal that commits while a reapply waits must count toward the limit: with two withdrawn
// applications and a live one being withdrawn, the reapply sees three and creates nothing.
func TestApplicationRepositoryIntegration_ReapplyCountsConcurrentWithdrawal(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	oldID, liveID := "00000000-0000-0000-0000-000000000092", "00000000-0000-0000-0000-000000000093"
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES
		('00000000-0000-0000-0000-000000000091', $1, $2, 'mentee', 'withdrawn'),
		($3, $1, $2, 'mentee', 'withdrawn'),
		($4, $1, $2, 'mentee', 'pending')`, fixture.OpenTerm, fixture.UserID, oldID, liveID); err != nil {
		t.Fatalf("insert applications: %v", err)
	}

	withdrawal, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin withdrawal: %v", err)
	}
	defer func() { _ = withdrawal.Rollback(ctx) }()
	if _, err := withdrawal.Exec(ctx, `UPDATE applications SET status = 'withdrawn' WHERE id = $1`, liveID); err != nil {
		t.Fatalf("withdraw live application: %v", err)
	}

	reapplied := make(chan error, 1)
	go func() {
		input := models.ApplicationCreateInput{ID: "00000000-0000-0000-0000-000000000094", UserID: fixture.UserID, Role: models.ApplicationRoleMentee, Status: models.ApplicationStatusPending}
		_, err := NewApplicationRepository(pool).ReapplyWithTasks(ctx, oldID, fixture.OpenTerm, input, nil)
		reapplied <- err
	}()
	waitForLockWait(t, pool, reapplied)
	if err := withdrawal.Commit(ctx); err != nil {
		t.Fatalf("commit withdrawal: %v", err)
	}

	if err := <-reapplied; !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("reapply racing a withdrawal err = %v; want ErrIneligible", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM applications WHERE user_id = $1`, fixture.UserID).Scan(&count); err != nil {
		t.Fatalf("count applications: %v", err)
	}
	if count != 3 {
		t.Fatalf("applications = %d; want the 3 withdrawn and no reapplication", count)
	}
}

// waitForLockWait returns once another session is blocked on a row lock, failing if the
// operation reported on done finishes first or nothing blocks in time.
func waitForLockWait(t *testing.T, pool *pgxpool.Pool, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("operation finished without waiting for the lock: %v", err)
		default:
		}
		var waiting bool
		if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock')`).Scan(&waiting); err != nil {
			t.Fatalf("read lock waits: %v", err)
		}
		if waiting {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the operation to block on a lock")
}

// A transition is authorized against a status read before the write, so a review that lands in
// between must not be overwritten: the assignee reopening a task the reviewer just completed fails.
func TestTaskRepositoryIntegration_UpdateRejectsStaleSourceStatus(t *testing.T) {
	pool, _ := fileIntegrationPool(t)
	ctx := context.Background()
	tasks := NewTaskRepository(pool)
	if _, err := pool.Exec(ctx, `UPDATE tasks SET status = 'complete' WHERE id = $1`, fileTestTask); err != nil {
		t.Fatal(err)
	}

	authorized, reopen := models.TaskStatusSubmitted, models.TaskStatusInProgress
	if _, err := tasks.Update(ctx, fileTestTask, models.TaskUpdateInput{Status: &reopen, FromStatus: &authorized}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale reopen err = %v; want ErrConflict", err)
	}
	var status models.TaskStatus
	if err := pool.QueryRow(ctx, `SELECT status FROM tasks WHERE id = $1`, fileTestTask).Scan(&status); err != nil || status != models.TaskStatusComplete {
		t.Fatalf("status = %q (err %v); the completed review must stand", status, err)
	}

	authorized = models.TaskStatusComplete
	got, err := tasks.Update(ctx, fileTestTask, models.TaskUpdateInput{Status: &reopen, FromStatus: &authorized})
	if err != nil || got.Status != models.TaskStatusInProgress {
		t.Fatalf("current-source update = %+v, %v; want in_progress", got, err)
	}
}

// A withdrawn application is kept as history when its applicant reapplies, so its tasks and their
// files cannot change. The check reads the application itself, not the task's denormalised status.
func TestTaskRepositoryIntegration_WithdrawnApplicationTasksAreLocked(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	applicationID, taskID := "00000000-0000-0000-0000-000000000095", "00000000-0000-0000-0000-000000000096"
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'withdrawn')`, applicationID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status, file, application_status) VALUES ($1, $2, $3, $4, 'in_progress', 'evidence', 'pending')`, taskID, applicationID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	submitted := models.TaskStatusSubmitted
	if _, err := NewTaskRepository(pool).Update(ctx, taskID, models.TaskUpdateInput{Status: &submitted}); !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("update err = %v; want ErrStateLocked", err)
	}
	files := NewFileRepository(pool)
	if err := files.ReplaceTaskFile(ctx, domain.FileReplacement{RowID: taskID, Previous: ptrTo("evidence"), Next: "replacement"}); !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("replace file err = %v; want ErrStateLocked", err)
	}
	if err := files.ClearTaskFile(ctx, taskID, "evidence"); !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("clear file err = %v; want ErrStateLocked", err)
	}

	var status, file string
	if err := pool.QueryRow(ctx, `SELECT status, file FROM tasks WHERE id = $1`, taskID).Scan(&status, &file); err != nil {
		t.Fatalf("read task: %v", err)
	}
	if status != string(models.TaskStatusInProgress) || file != "evidence" {
		t.Fatalf("task = status %q file %q; want it unchanged", status, file)
	}
}

// Reapply ordering and models.Application both rely on applications always carrying timestamps.
func TestApplicationRepositoryIntegration_TimestampsAreRequired(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	var pgErr *pgconn.PgError
	for _, column := range []string{"created_on", "updated_on"} {
		_, err := pool.Exec(context.Background(), `INSERT INTO applications (id, program_term_id, user_id, role, status, `+column+`)
			VALUES ('00000000-0000-0000-0000-000000000087', $1, $2, 'mentee', 'withdrawn', NULL)`, fixture.OpenTerm, fixture.UserID)
		if !errors.As(err, &pgErr) || pgErr.Code != "23502" {
			t.Fatalf("insert with NULL %s err = %v; want not_null_violation", column, err)
		}
	}
}

func TestDirectoryIntegration_NonStringSkillsAreDropped(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	skillSet := `{"skills":[null,"Rust",{"name":"Kubernetes"},7,"Go"]}`
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000080', $1, $2, 'mentee', 'accepted')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert accepted mentee application: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_members (id, program_id, user_id, member_type, status) VALUES ('00000000-0000-0000-0000-000000000081', $1, $2, 'mentor', 'approved')`, fixture.ProgramID, fixture.UserID); err != nil {
		t.Fatalf("insert active mentor membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_profiles (id, user_id, profile_type, skill_set) VALUES ('00000000-0000-0000-0000-000000000082', $1, 'mentee', $2), ('00000000-0000-0000-0000-000000000083', $1, 'mentor', $2)`, fixture.UserID, skillSet); err != nil {
		t.Fatalf("insert profiles: %v", err)
	}
	want := []string{"Go", "Rust"}

	mentees, err := NewMenteeRepository(pool).List(ctx, models.MenteeFilter{Limit: 10})
	if err != nil {
		t.Fatalf("mentee List: %v", err)
	}
	if len(mentees.Data) != 1 || !slices.Equal(mentees.Data[0].Skills, want) {
		t.Errorf("mentee list = %+v, want one mentee with skills %v", mentees.Data, want)
	}
	mentee, err := NewMenteeRepository(pool).GetByUserID(ctx, fixture.UserID)
	if err != nil {
		t.Fatalf("mentee GetByUserID: %v", err)
	}
	if !slices.Equal(mentee.Skills, want) {
		t.Errorf("mentee detail skills = %v, want %v", mentee.Skills, want)
	}

	mentors, err := NewMentorRepository(pool).List(ctx, models.MentorFilter{Limit: 10})
	if err != nil {
		t.Fatalf("mentor List: %v", err)
	}
	if len(mentors.Data) != 1 || !slices.Equal(mentors.Data[0].Skills, want) {
		t.Errorf("mentor list = %+v, want one mentor with skills %v", mentors.Data, want)
	}
	mentor, err := NewMentorRepository(pool).GetByUserID(ctx, fixture.UserID)
	if err != nil {
		t.Fatalf("mentor GetByUserID: %v", err)
	}
	if !slices.Equal(mentor.Skills, want) {
		t.Errorf("mentor detail skills = %v, want %v", mentor.Skills, want)
	}
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

func TestApplicationRepositoryIntegration_ListByProgramStatusFilterAndOrder(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	// Applicants in their expected order. Across ranks created_on rises down the list, so recency
	// alone would reverse it, and ids rise too, so the id tiebreaker alone would as well. Within the
	// two pending ranks the newer application comes first.
	applicants := []struct {
		id, status     string
		tasksSubmitted bool
		prerequisite   string // status of its one prerequisite task, or "" for none
		day            int
	}{
		{"00000000-0000-0000-0000-0000000000c1", "graduated", false, "", 0},
		{"00000000-0000-0000-0000-0000000000c2", "accepted", false, "", 1},
		{"00000000-0000-0000-0000-0000000000c3", "pending", false, "submitted", 3}, // tasks_submitted, not yet flagged
		{"00000000-0000-0000-0000-0000000000c4", "pending", false, "", 2},          // tasks_submitted: none to submit
		{"00000000-0000-0000-0000-0000000000c5", "pending", true, "incomplete", 5}, // applied: flagged, then reset by a reviewer
		{"00000000-0000-0000-0000-0000000000c6", "pending", false, "in_progress", 4},
		{"00000000-0000-0000-0000-0000000000c8", "declined", false, "", 7},
		{"00000000-0000-0000-0000-0000000000c9", "withdrawn", false, "", 8},
	}
	for _, a := range applicants {
		suffix := a.id[len(a.id)-1:]
		userID := "00000000-0000-0000-0000-0000000000d" + suffix
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ($1, $2, $2)`, userID, "applicant-"+suffix); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status, tasks_submitted, created_on)
			VALUES ($1, $2, $3, 'mentee', $4, $5, '2026-01-01T00:00:00Z'::timestamptz + make_interval(days => $6))`,
			a.id, fixture.OpenTerm, userID, a.status, a.tasksSubmitted, a.day); err != nil {
			t.Fatalf("insert %s application: %v", a.status, err)
		}
		if a.prerequisite != "" {
			if _, err := pool.Exec(ctx, `INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status, category, application_status, program_term_status)
				VALUES ($1, $2, $3, $4, $5, 'prerequisite', 'pending', 'open')`,
				"00000000-0000-0000-0000-0000000000e"+suffix, a.id, fixture.OpenTerm, userID, a.prerequisite); err != nil {
				t.Fatalf("insert prerequisite task: %v", err)
			}
		}
	}
	repo := NewApplicationRepository(pool)

	rows, _, err := repo.ListByProgram(ctx, fixture.ProgramID, models.ProgramApplicationFilter{Limit: 50})
	if err != nil || len(rows) != len(applicants) {
		t.Fatalf("ListByProgram = %d rows, err = %v; want %d", len(rows), err, len(applicants))
	}
	for i, row := range rows {
		if want := applicants[i].id; row.ApplicationID != want {
			t.Fatalf("row %d = %q (%s); want %q", i, row.ApplicationID, row.Status, want)
		}
	}

	for status, want := range map[models.ProgramApplicationStatus][]string{
		models.ProgramApplicationStatusApplied:                           {"00000000-0000-0000-0000-0000000000c5", "00000000-0000-0000-0000-0000000000c6"},
		models.ProgramApplicationStatusTasksSubmitted:                    {"00000000-0000-0000-0000-0000000000c3", "00000000-0000-0000-0000-0000000000c4"},
		models.ProgramApplicationStatus(models.ApplicationStatusPending): {"00000000-0000-0000-0000-0000000000c3", "00000000-0000-0000-0000-0000000000c4", "00000000-0000-0000-0000-0000000000c5", "00000000-0000-0000-0000-0000000000c6"},
	} {
		rows, meta, err := repo.ListByProgram(ctx, fixture.ProgramID, models.ProgramApplicationFilter{Status: status, Limit: 50})
		if err != nil || meta.Total != len(want) || len(rows) != len(want) {
			t.Fatalf("ListByProgram(status %q) = %d rows, meta %+v, err = %v; want %v", status, len(rows), meta, err, want)
		}
		for i, row := range rows {
			if row.ApplicationID != want[i] {
				t.Fatalf("ListByProgram(status %q) row %d = %q; want %q", status, i, row.ApplicationID, want[i])
			}
		}
	}
}

func TestApplicationRepositoryIntegration_ListByProgramFallsBackToMenteeProfile(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	const (
		bareUser    = "00000000-0000-0000-0000-0000000000f1" // signed in without name or email claims
		namedUser   = "00000000-0000-0000-0000-0000000000f2"
		bareApp     = "00000000-0000-0000-0000-0000000000f3"
		namedApp    = "00000000-0000-0000-0000-0000000000f4"
		profileMail = "ada+profile@example.org"
	)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid) VALUES ($1, 'bare')`, bareUser); err != nil {
		t.Fatalf("insert bare user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name, email) VALUES ($1, 'named', 'Grace Hopper', 'grace@example.org')`, namedUser); err != nil {
		t.Fatalf("insert named user: %v", err)
	}
	for _, p := range []struct{ userID, first, last, email string }{
		{bareUser, "Ada", "Lovelace", profileMail},
		{namedUser, "Profile", "Name", "profile@example.org"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO user_profiles (id, user_id, profile_type, first_name, last_name, email)
			VALUES (gen_random_uuid(), $1, 'mentee', $2, $3, $4)`, p.userID, p.first, p.last, p.email); err != nil {
			t.Fatalf("insert mentee profile: %v", err)
		}
	}
	for _, a := range []struct{ id, userID string }{{bareApp, bareUser}, {namedApp, namedUser}} {
		if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'pending')`,
			a.id, fixture.OpenTerm, a.userID); err != nil {
			t.Fatalf("insert application: %v", err)
		}
	}
	repo := NewApplicationRepository(pool)

	rows, _, err := repo.ListByProgram(ctx, fixture.ProgramID, models.ProgramApplicationFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListByProgram: %v", err)
	}
	want := map[string][2]string{bareApp: {"Ada Lovelace", profileMail}, namedApp: {"Grace Hopper", "grace@example.org"}}
	for _, row := range rows {
		w, ok := want[row.ApplicationID]
		if !ok {
			continue
		}
		if row.Name == nil || *row.Name != w[0] || row.Email == nil || *row.Email != w[1] {
			t.Fatalf("row %s name/email = %v/%v; want %q/%q", row.ApplicationID, row.Name, row.Email, w[0], w[1])
		}
		delete(want, row.ApplicationID)
	}
	if len(want) != 0 {
		t.Fatalf("ListByProgram is missing applications %v", want)
	}

	rows, meta, err := repo.ListByProgram(ctx, fixture.ProgramID, models.ProgramApplicationFilter{Search: "ada+profile", Limit: 50})
	if err != nil || meta.Total != 1 || len(rows) != 1 || rows[0].ApplicationID != bareApp {
		t.Fatalf("ListByProgram(search profile email) = %d rows, meta %+v, err = %v; want only %q", len(rows), meta, err, bareApp)
	}
}

func indexOutboxFixtureRecord(objectType, objectUID string) domain.IndexOutboxRecord {
	return domain.IndexOutboxRecord{ObjectType: objectType, ObjectUID: objectUID, Action: "updated", Headers: []byte(`{}`), Data: []byte(`{}`), IndexingConfig: []byte(`{}`)}
}

// seedSecondOpenTerm adds another open, in-window term to the fixture program and returns its id.
func seedSecondOpenTerm(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	termID := "00000000-0000-0000-0000-000000000013"
	if _, err := pool.Exec(context.Background(), `INSERT INTO program_terms (id, program_id, name, status, application_start_date, application_end_date, start_date_time, end_date_time)
		SELECT $1, program_id, 'Second', 'open', NOW() - interval '1 day', NOW() + interval '30 days', NOW() + interval '40 days', NOW() + interval '120 days'
		FROM program_terms WHERE id = '00000000-0000-0000-0000-000000000011'`, termID); err != nil {
		t.Fatalf("insert second term: %v", err)
	}
	return termID
}

// A mentee holds one accepted mentorship at a time, counting only terms that have not ended, and
// cannot apply once graduated. As in legacy, graduating does not stop an earlier application from
// being accepted.
func TestApplicationRepositoryIntegration_MenteeHoldsOneMentorship(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		other                        models.ApplicationStatus
		otherTermEnded               bool
		acceptRefused, createRefused bool
	}{
		{name: "accepted in a running term", other: models.ApplicationStatusAccepted, acceptRefused: true, createRefused: true},
		{name: "accepted in an ended term", other: models.ApplicationStatusAccepted, otherTermEnded: true},
		{name: "graduated", other: models.ApplicationStatusGraduated, createRefused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := integrationPool(t)
			fixture := seedIntegrationFixture(t, pool)
			second := seedSecondOpenTerm(t, pool)
			ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
			repo := NewApplicationRepository(pool)
			if tc.otherTermEnded {
				if _, err := pool.Exec(ctx, `UPDATE program_terms SET end_date_time = NOW() - interval '1 day' WHERE id = $1`, second); err != nil {
					t.Fatalf("end second term: %v", err)
				}
			}

			pendingID := "00000000-0000-0000-0000-0000000000f1"
			if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'pending'), ('00000000-0000-0000-0000-0000000000f2', $4, $3, 'mentee', $5)`,
				pendingID, fixture.OpenTerm, fixture.UserID, second, tc.other); err != nil {
				t.Fatalf("insert applications: %v", err)
			}

			accepted, pending := models.ApplicationStatusAccepted, models.ApplicationStatusPending
			attendance := models.AttendanceTypeFullTime
			_, err := repo.Update(ctx, pendingID, models.ApplicationUpdateInput{Status: &accepted, ExpectedStatus: &pending, AttendanceType: &attendance})
			if refused := errors.Is(err, domain.ErrIneligible); refused != tc.acceptRefused || (err != nil && !refused) {
				t.Fatalf("accept err = %v; want refused = %v", err, tc.acceptRefused)
			}
			if _, err := pool.Exec(ctx, `DELETE FROM applications WHERE id = $1`, pendingID); err != nil {
				t.Fatalf("delete first application: %v", err)
			}
			input := models.ApplicationCreateInput{ID: "00000000-0000-0000-0000-0000000000f3", UserID: fixture.UserID, Role: models.ApplicationRoleMentee, Status: models.ApplicationStatusPending}
			_, err = repo.Create(ctx, fixture.OpenTerm, input)
			if refused := errors.Is(err, domain.ErrIneligible); refused != tc.createRefused || (err != nil && !refused) {
				t.Fatalf("create err = %v; want refused = %v", err, tc.createRefused)
			}
		})
	}
}

// Two acceptances of the same mentee in different terms lock different term rows; the per-mentee
// lock still lets only one of them through.
func TestApplicationRepositoryIntegration_ConcurrentAcceptancesOfOneMentee(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	second := seedSecondOpenTerm(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	ids := []string{"00000000-0000-0000-0000-0000000000f4", "00000000-0000-0000-0000-0000000000f5"}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $3, $5, 'mentee', 'pending'), ($2, $4, $5, 'mentee', 'pending')`,
		ids[0], ids[1], fixture.OpenTerm, second, fixture.UserID); err != nil {
		t.Fatalf("insert applications: %v", err)
	}

	repo := NewApplicationRepository(pool)
	accepted, pending := models.ApplicationStatusAccepted, models.ApplicationStatusPending
	attendance := models.AttendanceTypeFullTime
	start := make(chan struct{})
	errs := make(chan error, len(ids))
	for _, id := range ids {
		go func() {
			<-start
			_, err := repo.Update(ctx, id, models.ApplicationUpdateInput{Status: &accepted, ExpectedStatus: &pending, AttendanceType: &attendance})
			errs <- err
		}()
	}
	close(start)
	succeeded, refused := 0, 0
	for range ids {
		switch err := <-errs; {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrIneligible):
			refused++
		default:
			t.Fatalf("accept: %v", err)
		}
	}
	if succeeded != 1 || refused != 1 {
		t.Fatalf("succeeded %d, refused %d; want exactly one acceptance", succeeded, refused)
	}
}

// The window is re-checked under the term lock, and the application's dates come from its term.
func TestApplicationRepositoryIntegration_CreateUsesTermWindowAndDates(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	repo := NewApplicationRepository(pool)
	if _, err := pool.Exec(ctx, `UPDATE program_terms SET start_date_time = '2027-01-10T00:00:00Z', end_date_time = '2027-04-10T00:00:00Z' WHERE id = $1`, fixture.OpenTerm); err != nil {
		t.Fatalf("set term dates: %v", err)
	}

	input := models.ApplicationCreateInput{ID: "00000000-0000-0000-0000-0000000000f6", UserID: fixture.UserID, Role: models.ApplicationRoleMentee, Status: models.ApplicationStatusPending}
	a, err := repo.Create(ctx, fixture.OpenTerm, input)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.StartDateTime == nil || !a.StartDateTime.Equal(time.Date(2027, 1, 10, 0, 0, 0, 0, time.UTC)) ||
		a.EndDateTime == nil || !a.EndDateTime.Equal(time.Date(2027, 4, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("application dates %v - %v; want the term's", a.StartDateTime, a.EndDateTime)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM applications WHERE id = $1`, input.ID); err != nil {
		t.Fatalf("delete application: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE program_terms SET application_end_date = NOW() - interval '2 days' WHERE id = $1`, fixture.OpenTerm); err != nil {
		t.Fatalf("close window: %v", err)
	}
	if _, err := repo.Create(ctx, fixture.OpenTerm, input); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("create after the window err = %v; want ErrIneligible", err)
	}
}

func TestApplicationRepositoryIntegration_EditingAcceptedMentorKeepsRemoval(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	appID := "00000000-0000-0000-0000-000000000091"
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentor', 'pending')`, appID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert mentor application: %v", err)
	}
	apps := NewApplicationRepository(pool)
	members := NewProgramMemberRepository(pool)
	memberState := func() (status, operation string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT status FROM program_members WHERE program_id = $1 AND user_id = $2 AND member_type = 'mentor'`, fixture.ProgramID, fixture.UserID).Scan(&status); err != nil {
			t.Fatalf("read mentor membership: %v", err)
		}
		if err := pool.QueryRow(ctx, `SELECT desired_operation FROM fga_outbox WHERE marker_kind = 'membership' AND object_uid = $1 AND relation = 'mentor'`, fixture.ProgramID).Scan(&operation); err != nil {
			t.Fatalf("read mentor membership marker: %v", err)
		}
		return status, operation
	}

	// Accepting the application makes the applicant an approved mentor.
	accepted, pending := models.ApplicationStatusAccepted, models.ApplicationStatusPending
	if _, err := apps.Update(ctx, appID, models.ApplicationUpdateInput{Status: &accepted, ExpectedStatus: &pending}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if status, operation := memberState(); status != string(models.ProgramMemberStatusApproved) || operation != "sync" {
		t.Fatalf("after acceptance: member %q, marker %q; want approved, sync", status, operation)
	}

	// An admin removes the mentor through the member row; the application stays accepted.
	var memberID string
	if err := pool.QueryRow(ctx, `SELECT id FROM program_members WHERE program_id = $1 AND user_id = $2 AND member_type = 'mentor'`, fixture.ProgramID, fixture.UserID).Scan(&memberID); err != nil {
		t.Fatalf("read member id: %v", err)
	}
	withdrawn := models.ProgramMemberStatusWithdrawn
	if _, err := members.Update(ctx, memberID, models.ProgramMemberUpdateInput{Status: &withdrawn}); err != nil {
		t.Fatalf("withdraw mentor: %v", err)
	}

	// Content-only edits of the accepted application leave the removal in place.
	note, evaluation := "keep notes", "partial"
	for name, input := range map[string]models.ApplicationUpdateInput{
		"empty":      {},
		"note":       {ReviewerNote: &note},
		"evaluation": {Evaluation: &evaluation},
	} {
		if _, err := apps.Update(ctx, appID, input); err != nil {
			t.Fatalf("%s edit: %v", name, err)
		}
		if status, operation := memberState(); status != string(models.ProgramMemberStatusWithdrawn) || operation != "remove" {
			t.Fatalf("after %s edit: member %q, marker %q; want withdrawn, remove", name, status, operation)
		}
	}
}
