// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

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
