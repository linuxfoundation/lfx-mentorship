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

func TestApplicationRepositoryIntegration_MarkTasksSubmittedFlipsOnce(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	applicationID := "00000000-0000-0000-0000-000000000071"
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'pending')`, applicationID, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatalf("insert application: %v", err)
	}
	repo := NewApplicationRepository(pool)

	for i, want := range []bool{true, false} {
		flipped, err := repo.MarkTasksSubmitted(ctx, applicationID)
		if err != nil || flipped != want {
			t.Fatalf("call %d: flipped = %v, err = %v; want %v", i+1, flipped, err, want)
		}
	}
	var indexed bool
	if err := pool.QueryRow(ctx, `SELECT (data->>'tasks_submitted')::boolean FROM index_outbox WHERE object_type = 'mentorship_application' AND object_uid = $1`, applicationID).Scan(&indexed); err != nil || !indexed {
		t.Fatalf("indexed tasks_submitted = %v, err = %v; want one refreshed document", indexed, err)
	}
	if flipped, err := repo.MarkTasksSubmitted(ctx, "00000000-0000-0000-0000-000000000072"); err != nil || flipped {
		t.Fatalf("missing application: flipped = %v, err = %v; want false, nil", flipped, err)
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
