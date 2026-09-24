// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_DSN: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = "mentorship,public"
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE tasks, applications, program_members, program_skills, program_terms, programs, users, fga_outbox, index_outbox RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("reset integration tables: %v", err)
	}
	return pool
}

type integrationFixture struct {
	UserID     string
	ProgramID  string
	OpenTerm   string
	ClosedTerm string
}

func seedIntegrationFixture(t *testing.T, pool *pgxpool.Pool) integrationFixture {
	t.Helper()
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	fixture := integrationFixture{UserID: "00000000-0000-0000-0000-000000000001", ProgramID: "00000000-0000-0000-0000-000000000010", OpenTerm: "00000000-0000-0000-0000-000000000011", ClosedTerm: "00000000-0000-0000-0000-000000000012"}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ($1, 'fixture-user', 'Fixture User')`, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO programs (id, project_uid, name, slug, status) VALUES ($1, '00000000-0000-0000-0000-000000000099', 'Fixture Program', 'fixture-program', 'published')`, fixture.ProgramID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_members (id, program_id, user_id, member_type, status) VALUES ('00000000-0000-0000-0000-000000000020', $1, $2, 'program_admin', 'active')`, fixture.ProgramID, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_terms (id, program_id, name, status) VALUES ($1, $3, 'Open', 'open'), ($2, $3, 'Closed', 'closed')`, fixture.OpenTerm, fixture.ClosedTerm, fixture.ProgramID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestFGAOutboxIntegration_ClaimSerializesSameObject(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-1", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	start := make(chan struct{})
	results := make(chan []domain.FGAOutboxMarker, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			markers, err := repo.Claim(ctx, 10)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			results <- markers
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	claimed := 0
	for markers := range results {
		claimed += len(markers)
	}
	if claimed != 1 {
		t.Fatalf("claimed %d markers; want exactly one", claimed)
	}
}

func TestIndexOutboxIntegration_ClaimRetryAndSent(t *testing.T) {
	pool := integrationPool(t)
	seedIntegrationFixture(t, pool)
	repo := NewIndexOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.Enqueue(ctx, domain.IndexOutboxRecord{ObjectType: "mentorship_program", ObjectUID: "00000000-0000-0000-0000-000000000010", Action: "updated", Headers: []byte(`{"authorization":"Bearer fixture"}`), Data: []byte(`{"id":"00000000-0000-0000-0000-000000000010"}`), IndexingConfig: []byte(`{"object_id":"00000000-0000-0000-0000-000000000010"}`)}); err != nil {
		t.Fatalf("enqueue index record: %v", err)
	}
	claimed, err := repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim records=%d err=%v", len(claimed), err)
	}
	if acknowledged, err := repo.MarkRetry(ctx, claimed[0]); err != nil {
		t.Fatalf("retry: %v", err)
	} else if !acknowledged {
		t.Fatal("retry acknowledgement missing")
	}
	claimed, err = repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("reclaim records=%d err=%v", len(claimed), err)
	}
	if acknowledged, err := repo.MarkSent(ctx, claimed[0]); err != nil {
		t.Fatalf("mark sent: %v", err)
	} else if !acknowledged {
		t.Fatal("sent acknowledgement missing")
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM index_outbox WHERE id = $1`, claimed[0].ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "sent" {
		t.Fatalf("state=%q; want sent", state)
	}
	var headerAuth string
	if err := pool.QueryRow(ctx, `SELECT headers->>'authorization' FROM index_outbox WHERE id = $1`, claimed[0].ID).Scan(&headerAuth); err != nil {
		t.Fatal(err)
	}
	if headerAuth != "present" {
		t.Fatalf("authorization header=%q; want present", headerAuth)
	}
}

func TestIndexOutboxIntegration_MarkSentRequeuesNewerGeneration(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	repo := NewIndexOutboxRepository(pool)
	ctx := context.Background()
	record := domain.IndexOutboxRecord{
		ObjectType:     "mentorship_program",
		ObjectUID:      fixture.ProgramID,
		Action:         "updated",
		Headers:        []byte(`{"authorization":"******"}`),
		Data:           []byte(`{"id":"` + fixture.ProgramID + `","name":"before"}`),
		IndexingConfig: []byte(`{"object_id":"` + fixture.ProgramID + `"}`),
	}
	if err := repo.Enqueue(ctx, record); err != nil {
		t.Fatalf("enqueue index record: %v", err)
	}
	claimed, err := repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim records=%d err=%v", len(claimed), err)
	}
	record.Data = []byte(`{"id":"` + fixture.ProgramID + `","name":"after"}`)
	if err := repo.Enqueue(ctx, record); err != nil {
		t.Fatalf("enqueue newer generation: %v", err)
	}
	if acknowledged, err := repo.MarkSent(ctx, claimed[0]); err != nil {
		t.Fatalf("mark sent: %v", err)
	} else if !acknowledged {
		t.Fatal("expected newer generation to be requeued")
	}
	var state string
	var generation int64
	var claimedGeneration *int64
	if err := pool.QueryRow(ctx, `SELECT state, generation, claimed_generation FROM index_outbox WHERE id = $1`, claimed[0].ID).Scan(&state, &generation, &claimedGeneration); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || generation != 2 || claimedGeneration != nil {
		t.Fatalf("state=%q generation=%d claimed_generation=%v", state, generation, claimedGeneration)
	}
}

func TestProgramHeaderProjectionIntegration_CountsProgramState(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ('00000000-0000-0000-0000-000000000002', 'fixture-user-2', 'Fixture User 2')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_members (id, program_id, user_id, member_type, status) VALUES ('00000000-0000-0000-0000-000000000021', $1, $2, 'mentor', 'active')`, fixture.ProgramID, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000030', $1, $2, 'mentee', 'accepted'), ('00000000-0000-0000-0000-000000000031', $1, '00000000-0000-0000-0000-000000000002', 'mentee', 'graduated')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	projection, err := NewProgramRepository(pool).GetHeaderProjection(ctx, fixture.ProgramID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ActiveTerm == nil || projection.ActiveTerm.ID != fixture.OpenTerm {
		t.Fatalf("active term=%#v", projection.ActiveTerm)
	}
	if projection.Stats.Mentors != 1 || projection.Stats.Mentees != 1 || projection.Stats.Graduated != 1 {
		t.Fatalf("stats=%+v", projection.Stats)
	}
}

func TestProgramHeaderProjectionIntegration_LeavesActiveTermNilWithoutOpenTerm(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE program_terms SET status = 'closed' WHERE id = $1`, fixture.OpenTerm); err != nil {
		t.Fatal(err)
	}
	projection, err := NewProgramRepository(pool).GetHeaderProjection(ctx, fixture.ProgramID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ActiveTerm != nil {
		t.Fatalf("active term=%#v; want nil", projection.ActiveTerm)
	}
}

func TestProgramTermDeleteIntegration_BlocksWhenApplicationsExist(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000050', $1, $2, 'mentee', 'pending')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	err := NewProgramTermRepository(pool).Delete(ctx, fixture.OpenTerm)
	if !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("delete error=%v; want ErrStateLocked", err)
	}
}

func TestUserProfileCreateAndUpsertOnExistingProfile(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_profiles (id, user_id, profile_type, slug, first_name, last_name, terms_and_conditions)
		VALUES
			('00000000-0000-0000-0000-000000000060', $1, 'mentor', 'mentor-one', 'One', 'User', true)
	`, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	repo := NewUserProfileRepository(pool)
	_, err := repo.Create(ctx, models.UserProfileCreateInput{
		ID:                 "00000000-0000-0000-0000-000000000061",
		UserID:             fixture.UserID,
		ProfileType:        "mentor",
		Slug:               stringPtr("mentor-duplicate"),
		FirstName:          stringPtr("Duplicate"),
		LastName:           stringPtr("User"),
		TermsAndConditions: true,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("create error=%v; want ErrConflict", err)
	}
	updated, inserted, err := repo.UpsertByUserAndType(ctx, models.UserProfileCreateInput{
		ID:                 "00000000-0000-0000-0000-000000000062",
		UserID:             fixture.UserID,
		ProfileType:        "mentor",
		Slug:               stringPtr("mentor-canonical"),
		FirstName:          stringPtr("Canonical"),
		LastName:           stringPtr("User"),
		TermsAndConditions: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inserted || updated.ID != "00000000-0000-0000-0000-000000000060" || updated.Slug == nil || *updated.Slug != "mentor-canonical" || updated.FirstName == nil || *updated.FirstName != "Canonical" {
		t.Fatalf("updated=%+v inserted=%v; want existing profile updated", updated, inserted)
	}
}

func stringPtr(value string) *string {
	return &value
}

func TestTermManagementIntegration_CountsApplicationStatuses(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ('00000000-0000-0000-0000-000000000003', 'fixture-user-3', 'Fixture User 3'), ('00000000-0000-0000-0000-000000000004', 'fixture-user-4', 'Fixture User 4'), ('00000000-0000-0000-0000-000000000005', 'fixture-user-5', 'Fixture User 5')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000040', $1, $2, 'mentee', 'pending'), ('00000000-0000-0000-0000-000000000041', $1, '00000000-0000-0000-0000-000000000003', 'mentee', 'declined'), ('00000000-0000-0000-0000-000000000042', $1, '00000000-0000-0000-0000-000000000004', 'mentee', 'accepted'), ('00000000-0000-0000-0000-000000000043', $1, '00000000-0000-0000-0000-000000000005', 'mentee', 'graduated')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	rows, _, err := NewProgramTermRepository(pool).ListManagementByProgram(ctx, fixture.ProgramID, models.ProgramTermFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var open *models.ProgramTermManagementRow
	for _, row := range rows {
		if row.ID == fixture.OpenTerm {
			open = row
			break
		}
	}
	if open == nil {
		t.Fatal("open term missing")
	}
	if open.Pending != 1 || open.Declined != 1 || open.Accepted != 1 || open.Graduated != 1 {
		t.Fatalf("counts=%+v", open)
	}
}

func TestProgramApplicationsIntegration_ReturnsTaskCounts(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000050', $1, $2, 'mentee', 'accepted')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status) VALUES ('00000000-0000-0000-0000-000000000051', '00000000-0000-0000-0000-000000000050', $1, $2, 'submitted'), ('00000000-0000-0000-0000-000000000052', '00000000-0000-0000-0000-000000000050', $1, $2, 'incomplete')`, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	rows, _, err := NewApplicationRepository(pool).ListByProgram(ctx, fixture.ProgramID, models.ProgramApplicationFilter{Type: models.ProgramApplicationTypeAll, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TasksTotal != 2 || rows[0].TasksSubmitted != 1 {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestEnrollmentIntegration_RollsBackWhenSkillInsertFails(t *testing.T) {
	pool := integrationPool(t)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ('00000000-0000-0000-0000-000000000060', 'enroll-admin', 'Enroll Admin')`); err != nil {
		t.Fatal(err)
	}
	repo := NewProgramRepository(pool)
	projectUID := "00000000-0000-0000-0000-000000000099"
	_, err := repo.CreateEnrollment(ctx, models.ProgramEnrollmentInput{Program: models.ProgramCreateInput{ID: "00000000-0000-0000-0000-000000000061", CreatorUserID: "00000000-0000-0000-0000-000000000060", ProjectUID: &projectUID, Name: "Rollback", Slug: "rollback", Status: models.ProgramStatusDraft}, Skills: []string{"Go", "Go"}})
	if err == nil {
		t.Fatal("expected duplicate skill failure")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM programs WHERE id = '00000000-0000-0000-0000-000000000061'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("program persisted after rollback: %d", count)
	}
}

func TestFGAOutboxIntegration_NewGenerationCannotBeAcknowledgedByOldClaim(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-2", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	old := markers[0]

	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-2", "update_access"); err != nil {
		t.Fatalf("enqueue newer generation: %v", err)
	}
	acknowledged, err := repo.Acknowledge(ctx, old)
	if err != nil {
		t.Fatalf("acknowledge old generation: %v", err)
	}
	if acknowledged {
		t.Fatal("old generation was acknowledged after a newer enqueue")
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM fga_outbox WHERE id = $1`, old.ID).Scan(&state); err != nil {
		t.Fatalf("read newer generation state: %v", err)
	}
	if state != "pending" {
		t.Fatalf("newer generation state = %q; want pending", state)
	}

	fresh, err := repo.Claim(ctx, 1)
	if err != nil || len(fresh) != 1 {
		t.Fatalf("claim fresh generation: markers=%d err=%v", len(fresh), err)
	}
	if fresh[0].Generation <= old.Generation {
		t.Fatalf("generation = %d; want greater than %d", fresh[0].Generation, old.Generation)
	}
}

func TestFGAOutboxIntegration_StaleClaimCanBeReclaimed(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_task", "task-1", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fga_outbox SET claimed_at = NOW() - INTERVAL '6 minutes' WHERE id = $1`, markers[0].ID); err != nil {
		t.Fatalf("age claim: %v", err)
	}
	fresh, err := repo.Claim(ctx, 1)
	if err != nil || len(fresh) != 1 {
		t.Fatalf("reclaim stale marker: markers=%d err=%v", len(fresh), err)
	}
	if fresh[0].ID != markers[0].ID {
		t.Fatalf("reclaimed ID = %d; want %d", fresh[0].ID, markers[0].ID)
	}
}

func TestFGAOutboxIntegration_DeadLetterPreservesFailure(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_application", "application-1", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.DeadLetter(ctx, markers[0], "permanent builder failure"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}
	var state, lastError string
	if err := pool.QueryRow(ctx, `SELECT state, last_error FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state, &lastError); err != nil {
		t.Fatalf("read dead-letter marker: %v", err)
	}
	if state != "dead_letter" || lastError != "permanent builder failure" {
		t.Fatalf("state=%q last_error=%q; want dead_letter and preserved error", state, lastError)
	}
}

func TestFGAOutboxIntegration_ReplayDeadLetter(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-replay", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.DeadLetter(ctx, markers[0], "permanent failure"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}
	if err := repo.ReplayDeadLetter(ctx, markers[0].ID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	fresh, err := repo.Claim(ctx, 1)
	if err != nil || len(fresh) != 1 {
		t.Fatalf("claim replayed marker: markers=%d err=%v", len(fresh), err)
	}
	if fresh[0].ID != markers[0].ID || fresh[0].Attempts != 0 {
		t.Fatalf("replayed marker = %+v; want original ID and zero attempts", fresh[0])
	}
}

func TestFGAOutboxIntegration_AcknowledgeHandlesNullClaimedAt(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()

	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO fga_outbox (
			marker_kind, object_type, object_uid, desired_operation,
			state, generation, claimed_generation, claimed_at
		)
		VALUES ('object', 'mentorship_program', 'program-null-claim', 'update_access', 'in_flight', 1, 1, NULL)
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert null-claimed marker: %v", err)
	}

	acknowledged, err := repo.Acknowledge(ctx, domain.FGAOutboxMarker{ID: id, Generation: 1, ClaimedAt: nil})
	if err != nil {
		t.Fatalf("acknowledge with nil claimed_at: %v", err)
	}
	if !acknowledged {
		t.Fatal("expected acknowledge to delete matching marker with nil claimed_at")
	}

	var remaining int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM fga_outbox WHERE id = $1`, id).Scan(&remaining); err != nil {
		t.Fatalf("count marker after acknowledge: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining markers = %d; want 0", remaining)
	}
}

func TestFGAOutboxIntegration_ReconcileObjectPreservesDeadLetterState(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()

	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-dead-reconcile", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.DeadLetter(ctx, markers[0], "builder failure"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}

	if err := repo.ReconcileObject(ctx, "mentorship_program", "program-dead-reconcile", "update_access"); err != nil {
		t.Fatalf("reconcile object: %v", err)
	}

	var state, lastError string
	if err := pool.QueryRow(ctx, `SELECT state, last_error FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state, &lastError); err != nil {
		t.Fatalf("read reconciled marker: %v", err)
	}
	if state != "dead_letter" {
		t.Fatalf("state = %q; want dead_letter", state)
	}
	if lastError != "builder failure" {
		t.Fatalf("last_error = %q; want builder failure", lastError)
	}
}

func TestFGAOutboxIntegration_ReconcileMembershipPreservesDeadLetterState(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()

	if err := repo.EnqueueMembership(ctx, "mentorship_program", "program-dead-membership", "mentor", "alice"); err != nil {
		t.Fatalf("enqueue membership: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.DeadLetter(ctx, markers[0], "membership builder failure"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}

	if err := repo.ReconcileMembership(ctx, "mentorship_program", "program-dead-membership", "mentor", "alice"); err != nil {
		t.Fatalf("reconcile membership: %v", err)
	}

	var state, lastError string
	if err := pool.QueryRow(ctx, `SELECT state, last_error FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state, &lastError); err != nil {
		t.Fatalf("read reconciled membership marker: %v", err)
	}
	if state != "dead_letter" {
		t.Fatalf("state = %q; want dead_letter", state)
	}
	if lastError != "membership builder failure" {
		t.Fatalf("last_error = %q; want membership builder failure", lastError)
	}
}
