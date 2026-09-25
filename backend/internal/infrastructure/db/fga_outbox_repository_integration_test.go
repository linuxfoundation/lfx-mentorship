// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

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
	if _, err := pool.Exec(ctx, `INSERT INTO programs (id, lf_project_uid, name, slug, status) VALUES ($1, '00000000-0000-0000-0000-000000000099', 'Fixture Program', 'fixture-program', 'published')`, fixture.ProgramID); err != nil {
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
	claimedID := claimed[0].ID
	if acknowledged, deadLettered, err := repo.MarkRetry(ctx, claimed[0]); err != nil {
		t.Fatalf("retry: %v", err)
	} else if !acknowledged {
		t.Fatal("retry acknowledgement missing")
	} else if deadLettered {
		t.Fatal("first failure unexpectedly dead-lettered")
	}
	var state string
	var attempts int
	var nextAttemptAt time.Time
	if err := pool.QueryRow(ctx, `SELECT state, attempts, next_attempt_at FROM index_outbox WHERE id = $1`, claimed[0].ID).Scan(&state, &attempts, &nextAttemptAt); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || attempts != 1 || !nextAttemptAt.After(time.Now()) {
		t.Fatalf("state=%q attempts=%d next_attempt_at=%s; want pending, 1, and future retry", state, attempts, nextAttemptAt)
	}
	claimed, err = repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("claim before retry delay: records=%d err=%v", len(claimed), err)
	}
	if _, err := pool.Exec(ctx, `UPDATE index_outbox SET next_attempt_at = NOW() WHERE id = $1`, claimedID); err != nil {
		t.Fatal(err)
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

func TestIndexOutboxIntegration_ApplicationAndTaskLifecycle(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	applicationID := "00000000-0000-0000-0000-000000000030"
	taskID := "00000000-0000-0000-0000-000000000031"

	application, err := NewApplicationRepository(pool).Create(ctx, fixture.OpenTerm, models.ApplicationCreateInput{
		ID:     applicationID,
		UserID: fixture.UserID,
		Role:   models.ApplicationRoleMentee,
		Status: models.ApplicationStatusPending,
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}

	var applicationAction string
	var applicationConfig []byte
	if err := pool.QueryRow(ctx, `SELECT action, indexing_config FROM index_outbox WHERE object_type = 'mentorship_application' AND object_uid = $1`, application.ID).Scan(&applicationAction, &applicationConfig); err != nil {
		t.Fatalf("read application index record: %v", err)
	}
	var applicationConfigMap map[string]any
	if err := json.Unmarshal(applicationConfig, &applicationConfigMap); err != nil {
		t.Fatalf("decode application index config: %v", err)
	}
	if applicationAction != "created" || applicationConfigMap["object_ref"] != nil || applicationConfigMap["access_check_relation"] != "auditor" || applicationConfigMap["parent_refs"] == nil {
		t.Fatalf("application index action/config = %q/%v", applicationAction, applicationConfigMap)
	}

	name := "First task"
	category := models.TaskCategoryPrerequisite
	programTermID := fixture.OpenTerm
	if _, err := NewTaskRepository(pool).Create(ctx, application.ID, models.TaskCreateInput{
		ID:            taskID,
		ProgramTermID: &programTermID,
		AssigneeID:    fixture.UserID,
		Name:          &name,
		Category:      &category,
		Status:        models.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	var taskAction string
	var taskConfig []byte
	if err := pool.QueryRow(ctx, `SELECT action, indexing_config FROM index_outbox WHERE object_type = 'mentorship_task' AND object_uid = $1`, taskID).Scan(&taskAction, &taskConfig); err != nil {
		t.Fatalf("read task index record: %v", err)
	}
	var taskConfigMap map[string]any
	if err := json.Unmarshal(taskConfig, &taskConfigMap); err != nil {
		t.Fatalf("decode task index config: %v", err)
	}
	if taskAction != "created" || taskConfigMap["object_ref"] != nil || taskConfigMap["history_check_object"] != "mentorship_application:"+applicationID {
		t.Fatalf("task index action/config = %q/%v", taskAction, taskConfigMap)
	}
}

func TestProgramRepositoryIntegration_ListManagedByUser(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO programs (id, lf_project_uid, name, slug, status) VALUES ('00000000-0000-0000-0000-000000000013', '00000000-0000-0000-0000-000000000100', 'Inactive Program', 'inactive-program', 'published')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_members (id, program_id, user_id, member_type, status) VALUES ('00000000-0000-0000-0000-000000000022', '00000000-0000-0000-0000-000000000013', $1, 'program_admin', 'withdrawn')`, fixture.UserID); err != nil {
		t.Fatal(err)
	}

	programs, meta, err := NewProgramRepository(pool).ListManagedByUser(ctx, fixture.UserID, models.ProgramFilter{Limit: 10, Search: "fixture"})
	if err != nil {
		t.Fatalf("list managed programs: %v", err)
	}
	if len(programs) != 1 || programs[0].ID != fixture.ProgramID || meta.Total != 1 {
		t.Fatalf("programs/meta = %#v/%#v; want fixture program and total 1", programs, meta)
	}
}

func TestProgramIndexIntegration_RefreshesPublicStats(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})

	mentorID := "00000000-0000-0000-0000-000000000002"
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ($1, 'fixture-mentor', 'Fixture Mentor')`, mentorID); err != nil {
		t.Fatal(err)
	}
	graduatedUserID := "00000000-0000-0000-0000-000000000003"
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ($1, 'fixture-graduated', 'Fixture Graduated')`, graduatedUserID); err != nil {
		t.Fatal(err)
	}
	active := models.ProgramMemberStatusActive
	if _, err := NewProgramMemberRepository(pool).Create(ctx, fixture.ProgramID, models.ProgramMemberCreateInput{
		ID:         "00000000-0000-0000-0000-000000000021",
		UserID:     mentorID,
		MemberType: models.MemberTypeMentor,
		Status:     &active,
	}); err != nil {
		t.Fatalf("create mentor membership: %v", err)
	}

	applicationRepo := NewApplicationRepository(pool)
	acceptedApplication, err := applicationRepo.Create(ctx, fixture.OpenTerm, models.ApplicationCreateInput{
		ID:     "00000000-0000-0000-0000-000000000040",
		UserID: fixture.UserID,
		Role:   models.ApplicationRoleMentee,
		Status: models.ApplicationStatusPending,
	})
	if err != nil {
		t.Fatalf("create accepted application: %v", err)
	}
	accepted := models.ApplicationStatusAccepted
	if _, err := applicationRepo.Update(ctx, acceptedApplication.ID, models.ApplicationUpdateInput{Status: &accepted}); err != nil {
		t.Fatalf("accept application: %v", err)
	}

	graduatedApplication, err := applicationRepo.Create(ctx, fixture.OpenTerm, models.ApplicationCreateInput{
		ID:     "00000000-0000-0000-0000-000000000041",
		UserID: graduatedUserID,
		Role:   models.ApplicationRoleMentee,
		Status: models.ApplicationStatusPending,
	})
	if err != nil {
		t.Fatalf("create graduated application: %v", err)
	}
	if _, err := applicationRepo.Update(ctx, graduatedApplication.ID, models.ApplicationUpdateInput{Status: &accepted}); err != nil {
		t.Fatalf("accept graduated application: %v", err)
	}
	graduated := models.ApplicationStatusGraduated
	if _, err := applicationRepo.Update(ctx, graduatedApplication.ID, models.ApplicationUpdateInput{Status: &graduated}); err != nil {
		t.Fatalf("graduate application: %v", err)
	}

	var data []byte
	if err := pool.QueryRow(ctx, `SELECT data FROM index_outbox WHERE object_type = 'mentorship_program' AND object_uid = $1`, fixture.ProgramID).Scan(&data); err != nil {
		t.Fatalf("read program index snapshot: %v", err)
	}
	var document struct {
		Stats models.ProgramHeaderStats `json:"stats"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode program index snapshot: %v", err)
	}
	if document.Stats.Mentors != 1 || document.Stats.Mentees != 1 || document.Stats.Graduated != 1 {
		t.Fatalf("program stats = %+v; want mentors=1 mentees=1 graduated=1", document.Stats)
	}
}

func TestIndexOutboxIntegration_MarkRetryRequeuesNewerGeneration(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	repo := NewIndexOutboxRepository(pool)
	ctx := context.Background()
	record := domain.IndexOutboxRecord{
		ObjectType:     "mentorship_program",
		ObjectUID:      fixture.ProgramID,
		Action:         "updated",
		Data:           []byte(`{"id":"` + fixture.ProgramID + `","name":"before"}`),
		IndexingConfig: []byte(`{"object_id":"` + fixture.ProgramID + `"}`),
	}
	if err := repo.Enqueue(ctx, record); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim records=%d err=%v", len(claimed), err)
	}
	record.Data = []byte(`{"id":"` + fixture.ProgramID + `","name":"after"}`)
	if err := repo.Enqueue(ctx, record); err != nil {
		t.Fatalf("enqueue newer generation: %v", err)
	}
	if acknowledged, deadLettered, err := repo.MarkRetry(ctx, claimed[0]); err != nil || !acknowledged || deadLettered {
		t.Fatalf("mark retry: acknowledged=%v dead_lettered=%v err=%v", acknowledged, deadLettered, err)
	}
	var state string
	var attempts int
	var generation int64
	var claimedGeneration *int64
	if err := pool.QueryRow(ctx, `SELECT state, attempts, generation, claimed_generation FROM index_outbox WHERE id = $1`, claimed[0].ID).Scan(&state, &attempts, &generation, &claimedGeneration); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || attempts != 0 || generation != 2 || claimedGeneration != nil {
		t.Fatalf("state=%q attempts=%d generation=%d claimed_generation=%v", state, attempts, generation, claimedGeneration)
	}
}

func TestIndexOutboxIntegration_RequeueDeadLetter(t *testing.T) {
	pool := integrationPool(t)
	repo := NewIndexOutboxRepository(pool)
	repo.SetMaxAttempts(1)
	ctx := context.Background()
	record := domain.IndexOutboxRecord{
		ObjectType:     "mentorship_program",
		ObjectUID:      "00000000-0000-0000-0000-000000000010",
		Action:         "updated",
		Data:           []byte(`{"id":"00000000-0000-0000-0000-000000000010"}`),
		IndexingConfig: []byte(`{"object_id":"00000000-0000-0000-0000-000000000010"}`),
	}
	if err := repo.Enqueue(ctx, record); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim records=%d err=%v", len(claimed), err)
	}
	if acknowledged, deadLettered, err := repo.MarkRetry(ctx, claimed[0]); err != nil || !acknowledged || !deadLettered {
		t.Fatalf("dead-letter record: acknowledged=%v dead_lettered=%v err=%v", acknowledged, deadLettered, err)
	}
	requeued, err := repo.RequeueDeadLetter(ctx, record.ObjectType, record.ObjectUID)
	if err != nil || !requeued {
		t.Fatalf("requeue dead-letter: requeued=%v err=%v", requeued, err)
	}
	var state string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT state, attempts FROM index_outbox WHERE object_type = $1 AND object_uid = $2`, record.ObjectType, record.ObjectUID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || attempts != 0 {
		t.Fatalf("state=%q attempts=%d; want pending and 0", state, attempts)
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
	requeued, err := repo.RequeueDeadLetter(ctx, "mentorship_application", "application-1", "", "")
	if err != nil || !requeued {
		t.Fatalf("requeue dead-letter: requeued=%v err=%v", requeued, err)
	}
	var attempts int
	var clearedError *string
	if err := pool.QueryRow(ctx, `SELECT state, attempts, last_error FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state, &attempts, &clearedError); err != nil {
		t.Fatalf("read requeued marker: %v", err)
	}
	if state != "pending" || attempts != 0 || clearedError != nil {
		t.Fatalf("state=%q attempts=%d last_error=%v; want pending, 0, and nil", state, attempts, clearedError)
	}
}

func TestFGAOutboxIntegration_RequeueExactMembershipDeadLetter(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueMembershipRemoval(ctx, "mentorship_program", "program-1", "mentor", "fixture-user"); err != nil {
		t.Fatalf("enqueue removal: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.DeadLetter(ctx, markers[0], "dependency unavailable"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}
	if requeued, err := repo.RequeueDeadLetter(ctx, "mentorship_program", "program-1", "mentor", "other-user"); err != nil || requeued {
		t.Fatalf("wrong member requeue: requeued=%v err=%v", requeued, err)
	}
	if requeued, err := repo.RequeueDeadLetter(ctx, "mentorship_program", "program-1", "mentor", "fixture-user"); err != nil || !requeued {
		t.Fatalf("exact member requeue: requeued=%v err=%v", requeued, err)
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
