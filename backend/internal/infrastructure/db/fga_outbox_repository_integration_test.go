// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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

func TestApplicationRepositoryIntegration_UpdateRefreshesTaskStateAndIndex(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})

	applicationID := "00000000-0000-0000-0000-000000000080"
	application, err := NewApplicationRepository(pool).Create(ctx, fixture.OpenTerm, models.ApplicationCreateInput{
		ID:     applicationID,
		UserID: fixture.UserID,
		Role:   models.ApplicationRoleMentee,
		Status: models.ApplicationStatusPending,
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}

	taskID := "00000000-0000-0000-0000-000000000081"
	name := "Status follower"
	category := models.TaskCategoryPrerequisite
	programTermID := fixture.OpenTerm
	if _, err := NewTaskRepository(pool).Create(ctx, application.ID, models.TaskCreateInput{
		ID:            taskID,
		ProgramTermID: &programTermID,
		AssigneeID:    fixture.UserID,
		Name:          &name,
		Category:      &category,
		Status:        models.TaskStatusIncomplete,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	accepted := models.ApplicationStatusAccepted
	if _, err := NewApplicationRepository(pool).Update(ctx, application.ID, models.ApplicationUpdateInput{Status: &accepted}); err != nil {
		t.Fatalf("update application: %v", err)
	}

	var taskApplicationStatus models.ApplicationStatus
	if err := pool.QueryRow(ctx, `SELECT application_status FROM tasks WHERE id = $1`, taskID).Scan(&taskApplicationStatus); err != nil {
		t.Fatalf("read task application_status: %v", err)
	}
	if taskApplicationStatus != models.ApplicationStatusAccepted {
		t.Fatalf("task application_status=%q; want %q", taskApplicationStatus, models.ApplicationStatusAccepted)
	}

	var action string
	if err := pool.QueryRow(ctx, `SELECT action FROM index_outbox WHERE object_type = 'mentorship_task' AND object_uid = $1`, taskID).Scan(&action); err != nil {
		t.Fatalf("read task index action: %v", err)
	}
	if action != "updated" {
		t.Fatalf("task index action=%q; want updated", action)
	}
}

func TestApplicationTaskIntegration_CreateSnapshotsCanonicalLifecycleState(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})

	application, err := NewApplicationRepository(pool).Create(ctx, fixture.OpenTerm, models.ApplicationCreateInput{
		ID:     "00000000-0000-0000-0000-000000000082",
		UserID: fixture.UserID,
		Role:   models.ApplicationRoleMentee,
		Status: models.ApplicationStatusPending,
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	if application.ProgramTermStatus == nil || *application.ProgramTermStatus != models.ProgramTermStatusOpen {
		t.Fatalf("application program_term_status=%v; want open", application.ProgramTermStatus)
	}
	accepted := models.ApplicationStatusAccepted
	if _, err := NewApplicationRepository(pool).Update(ctx, application.ID, models.ApplicationUpdateInput{Status: &accepted}); err != nil {
		t.Fatalf("accept application: %v", err)
	}

	taskID := "00000000-0000-0000-0000-000000000083"
	name := "Created after acceptance"
	programTermID := fixture.OpenTerm
	task, err := NewTaskRepository(pool).Create(ctx, application.ID, models.TaskCreateInput{
		ID:            taskID,
		ProgramTermID: &programTermID,
		AssigneeID:    fixture.UserID,
		Name:          &name,
		Status:        models.TaskStatusIncomplete,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.ApplicationStatus == nil || *task.ApplicationStatus != models.ApplicationStatusAccepted || task.ProgramTermStatus == nil || *task.ProgramTermStatus != models.ProgramTermStatusOpen {
		t.Fatalf("task lifecycle=%v/%v; want accepted/open", task.ApplicationStatus, task.ProgramTermStatus)
	}

	type lifecycle struct {
		ApplicationStatus models.ApplicationStatus `json:"application_status"`
		ProgramTermStatus models.ProgramTermStatus `json:"program_term_status"`
	}
	snapshot := func(objectType, objectID string) lifecycle {
		t.Helper()
		var data []byte
		if err := pool.QueryRow(ctx, `SELECT data FROM index_outbox WHERE object_type = $1 AND object_uid = $2`, objectType, objectID).Scan(&data); err != nil {
			t.Fatalf("read %s index snapshot: %v", objectType, err)
		}
		var document lifecycle
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatalf("decode %s index snapshot: %v", objectType, err)
		}
		return document
	}
	if got := snapshot("mentorship_application", application.ID); got.ProgramTermStatus != models.ProgramTermStatusOpen {
		t.Fatalf("application snapshot program_term_status=%q; want open", got.ProgramTermStatus)
	}
	if got := snapshot("mentorship_task", taskID); got.ApplicationStatus != models.ApplicationStatusAccepted || got.ProgramTermStatus != models.ProgramTermStatusOpen {
		t.Fatalf("task snapshot lifecycle=%+v; want accepted/open", got)
	}
}

func TestProgramRepositoryIntegration_DeleteEnqueuesChildIndexDeletes(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})

	applicationID := "00000000-0000-0000-0000-000000000082"
	application, err := NewApplicationRepository(pool).Create(ctx, fixture.OpenTerm, models.ApplicationCreateInput{
		ID:     applicationID,
		UserID: fixture.UserID,
		Role:   models.ApplicationRoleMentee,
		Status: models.ApplicationStatusPending,
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}

	taskID := "00000000-0000-0000-0000-000000000083"
	name := "To be deleted with program"
	category := models.TaskCategoryPrerequisite
	programTermID := fixture.OpenTerm
	if _, err := NewTaskRepository(pool).Create(ctx, application.ID, models.TaskCreateInput{
		ID:            taskID,
		ProgramTermID: &programTermID,
		AssigneeID:    fixture.UserID,
		Name:          &name,
		Category:      &category,
		Status:        models.TaskStatusIncomplete,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := NewProgramRepository(pool).Delete(ctx, fixture.ProgramID); err != nil {
		t.Fatalf("delete program: %v", err)
	}

	var applicationAction string
	if err := pool.QueryRow(ctx, `SELECT action FROM index_outbox WHERE object_type = 'mentorship_application' AND object_uid = $1`, applicationID).Scan(&applicationAction); err != nil {
		t.Fatalf("read application index delete action: %v", err)
	}
	if applicationAction != "deleted" {
		t.Fatalf("application index action=%q; want deleted", applicationAction)
	}

	var taskAction string
	if err := pool.QueryRow(ctx, `SELECT action FROM index_outbox WHERE object_type = 'mentorship_task' AND object_uid = $1`, taskID).Scan(&taskAction); err != nil {
		t.Fatalf("read task index delete action: %v", err)
	}
	if taskAction != "deleted" {
		t.Fatalf("task index action=%q; want deleted", taskAction)
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

func TestProgramListAdministeredIntegration_GroupsStatusAndScopesToActiveAdmin(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	const (
		otherUserID   = "00000000-0000-0000-0000-000000000002"
		completedID   = "00000000-0000-0000-0000-000000000040"
		pendingID     = "00000000-0000-0000-0000-000000000041"
		mentorOnlyID  = "00000000-0000-0000-0000-000000000042"
		withdrawnID   = "00000000-0000-0000-0000-000000000043"
		rejectedID    = "00000000-0000-0000-0000-000000000060"
		hiddenID      = "00000000-0000-0000-0000-000000000061"
		archivedID    = "00000000-0000-0000-0000-000000000062"
		noTermsID     = "00000000-0000-0000-0000-000000000063"
		deletedTermID = "00000000-0000-0000-0000-000000000064"
		otherAdminID  = "00000000-0000-0000-0000-000000000065"
	)
	for _, q := range []string{
		`INSERT INTO users (id, lfid, name) VALUES ('` + otherUserID + `', 'fixture-user-2', 'Fixture User 2')`,
		`INSERT INTO programs (id, name, slug, status, lf_project_name) VALUES
			('` + completedID + `', 'Alpha', 'alpha', 'published', 'LF Energy'),
			('` + pendingID + `', 'beta', 'beta', 'submitted', 'CNCF'),
			('` + mentorOnlyID + `', 'Mentored', 'mentored', 'published', NULL),
			('` + withdrawnID + `', 'Withdrawn', 'withdrawn', 'published', NULL),
			('` + rejectedID + `', 'Gamma', 'gamma', 'rejected', NULL),
			('` + hiddenID + `', 'Hidden', 'hidden', 'hidden', NULL),
			('` + archivedID + `', 'Iota', 'iota', 'archived', NULL),
			('` + noTermsID + `', 'Kappa', 'kappa', 'published', NULL),
			('` + deletedTermID + `', 'Lambda', 'lambda', 'published', NULL),
			('` + otherAdminID + `', 'Other', 'other', 'pending', NULL)`,
		`INSERT INTO program_members (id, program_id, user_id, member_type, status) VALUES
			('00000000-0000-0000-0000-000000000044', '` + completedID + `', '` + fixture.UserID + `', 'program_admin', 'active'),
			('00000000-0000-0000-0000-000000000045', '` + pendingID + `', '` + fixture.UserID + `', 'program_admin', 'active'),
			('00000000-0000-0000-0000-000000000046', '` + mentorOnlyID + `', '` + fixture.UserID + `', 'mentor', 'active'),
			('00000000-0000-0000-0000-000000000047', '` + withdrawnID + `', '` + fixture.UserID + `', 'program_admin', 'withdrawn'),
			('00000000-0000-0000-0000-000000000048', '` + fixture.ProgramID + `', '` + fixture.UserID + `', 'mentor', 'active'),
			('00000000-0000-0000-0000-000000000066', '` + fixture.ProgramID + `', '` + otherUserID + `', 'mentor', 'withdrawn'),
			('00000000-0000-0000-0000-000000000067', '` + rejectedID + `', '` + fixture.UserID + `', 'program_admin', 'active'),
			('00000000-0000-0000-0000-000000000068', '` + hiddenID + `', '` + fixture.UserID + `', 'program_admin', 'active'),
			('00000000-0000-0000-0000-000000000069', '` + archivedID + `', '` + fixture.UserID + `', 'program_admin', 'active'),
			('00000000-0000-0000-0000-00000000006a', '` + noTermsID + `', '` + fixture.UserID + `', 'program_admin', 'active'),
			('00000000-0000-0000-0000-00000000006b', '` + deletedTermID + `', '` + fixture.UserID + `', 'program_admin', 'active'),
			('00000000-0000-0000-0000-00000000006c', '` + otherAdminID + `', '` + otherUserID + `', 'program_admin', 'active')`,
		`INSERT INTO program_terms (id, program_id, name, status, start_date_time) VALUES
			('00000000-0000-0000-0000-000000000049', '` + completedID + `', 'Spring 2026', 'closed', '2026-01-01'),
			('00000000-0000-0000-0000-00000000004a', '` + completedID + `', 'Summer 2026', 'closed', '2026-05-01'),
			('00000000-0000-0000-0000-00000000006d', '` + deletedTermID + `', 'Deleted', 'deleted', '2026-01-01')`,
		`INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES
			('00000000-0000-0000-0000-00000000004b', '` + fixture.OpenTerm + `', '` + fixture.UserID + `', 'mentee', 'accepted'),
			('00000000-0000-0000-0000-00000000006e', '` + fixture.OpenTerm + `', '` + otherUserID + `', 'mentee', 'graduated')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewProgramRepository(pool)

	programs, meta, err := repo.ListAdministeredByUser(ctx, fixture.UserID, models.AdministeredProgramFilter{})
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		id     string
		status models.AdministeredProgramStatus
		term   string
	}
	want := []row{
		{completedID, models.AdministeredProgramStatusCompleted, "Summer 2026"},
		{pendingID, models.AdministeredProgramStatusPendingReview, ""},
		{fixture.ProgramID, models.AdministeredProgramStatusOpen, "Open"},
		{rejectedID, models.AdministeredProgramStatusRejected, ""},
		{hiddenID, models.AdministeredProgramStatusHidden, ""},
		{archivedID, models.AdministeredProgramStatusHidden, ""},
		{noTermsID, models.AdministeredProgramStatusOpen, ""},
		{deletedTermID, models.AdministeredProgramStatusOpen, ""},
	}
	// Mentor-only and withdrawn memberships, and the other user's program, are excluded.
	if meta.Total != len(want) || len(programs) != len(want) {
		t.Fatalf("total=%d len=%d; want %d", meta.Total, len(programs), len(want))
	}
	for i, w := range want {
		p := programs[i]
		term := ""
		if p.Term != nil {
			term = p.Term.Name
			if p.Term.ProgramID != p.ID {
				t.Errorf("programs[%d].Term.ProgramID = %s; want %s", i, p.Term.ProgramID, p.ID)
			}
		}
		if p.ID != w.id || p.AdminStatus != w.status || term != w.term {
			t.Errorf("programs[%d] = {%s %s %q}; want %+v", i, p.ID, p.AdminStatus, term, w)
		}
		header, err := repo.GetHeaderProjection(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.Stats != header.Stats {
			t.Errorf("programs[%d] stats = %+v; want header stats %+v", i, p.Stats, header.Stats)
		}
	}
	if programs[0].Term.Status != models.ProgramTermStatusClosed || programs[2].Term.Status != models.ProgramTermStatusOpen {
		t.Errorf("term statuses = %s, %s; want closed, open", programs[0].Term.Status, programs[2].Term.Status)
	}
	if s := programs[2].Stats; s.Mentors != 1 || s.Mentees != 1 || s.Graduated != 1 {
		t.Errorf("fixture program stats = %+v; want 1 active mentor, 1 mentee, 1 graduated", s)
	}

	others, meta, err := repo.ListAdministeredByUser(ctx, otherUserID, models.AdministeredProgramFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Total != 1 || len(others) != 1 || others[0].ID != otherAdminID {
		t.Errorf("other user: total=%d programs=%+v; want only %s", meta.Total, others, otherAdminID)
	}

	for status, wantIDs := range map[models.AdministeredProgramStatus][]string{
		models.AdministeredProgramStatusCompleted: {completedID},
		models.AdministeredProgramStatusOpen:      {fixture.ProgramID, noTermsID, deletedTermID},
		models.AdministeredProgramStatusHidden:    {hiddenID, archivedID},
	} {
		got, meta, err := repo.ListAdministeredByUser(ctx, fixture.UserID, models.AdministeredProgramFilter{Status: status})
		if err != nil {
			t.Fatal(err)
		}
		gotIDs := make([]string, len(got))
		for i, p := range got {
			gotIDs[i] = p.ID
		}
		if meta.Total != len(wantIDs) || !slices.Equal(gotIDs, wantIDs) {
			t.Errorf("status=%s: total=%d ids=%v; want %v", status, meta.Total, gotIDs, wantIDs)
		}
	}

	byProject, meta, err := repo.ListAdministeredByUser(ctx, fixture.UserID, models.AdministeredProgramFilter{Search: "cncf"})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Total != 1 || len(byProject) != 1 || byProject[0].ID != pendingID {
		t.Errorf("search=cncf: total=%d programs=%+v; want only %s", meta.Total, byProject, pendingID)
	}

	paged, meta, err := repo.ListAdministeredByUser(ctx, fixture.UserID, models.AdministeredProgramFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Total != len(want) || len(paged) != 1 || paged[0].ID != pendingID {
		t.Errorf("limit=1 offset=1: total=%d programs=%+v; want %s of %d", meta.Total, paged, pendingID, len(want))
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

func TestProgramTermListIntegration_ExcludesDeletedTerms(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE program_terms SET status = 'deleted' WHERE id = $1`, fixture.ClosedTerm); err != nil {
		t.Fatal(err)
	}
	terms, meta, err := NewProgramTermRepository(pool).ListByProgram(ctx, fixture.ProgramID, models.ProgramTermFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Total != 1 || len(terms) != 1 || terms[0].ID != fixture.OpenTerm {
		t.Fatalf("terms=%v total=%d; want only the open term", terms, meta.Total)
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

	if _, err := repo.Create(ctx, models.UserProfileCreateInput{
		ID:                 "00000000-0000-0000-0000-000000000061",
		UserID:             fixture.UserID,
		ProfileType:        "mentor",
		Slug:               stringPtr("mentor-second"),
		FirstName:          stringPtr("Second"),
		LastName:           stringPtr("User"),
		TermsAndConditions: true,
	}); err != nil {
		t.Fatalf("create second mentor profile: %v; want success", err)
	}

	if _, err := repo.Create(ctx, models.UserProfileCreateInput{
		ID:                 "00000000-0000-0000-0000-000000000063",
		UserID:             fixture.UserID,
		ProfileType:        "mentee",
		Slug:               stringPtr("mentee-one"),
		TermsAndConditions: true,
	}); err != nil {
		t.Fatalf("create first mentee profile: %v", err)
	}
	_, err = repo.Create(ctx, models.UserProfileCreateInput{
		ID:                 "00000000-0000-0000-0000-000000000064",
		UserID:             fixture.UserID,
		ProfileType:        "mentee",
		Slug:               stringPtr("mentee-duplicate"),
		TermsAndConditions: true,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("create duplicate mentee error=%v; want ErrConflict", err)
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

func TestTaskIntegration_ApplicationParentIsRequired(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	_, err := pool.Exec(context.Background(), `INSERT INTO tasks (id, program_term_id, assignee_id, status) VALUES ('00000000-0000-0000-0000-000000000090', $1, $2, 'incomplete')`, fixture.OpenTerm, fixture.UserID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23502" || pgErr.ColumnName != "application_id" {
		t.Fatalf("insert without application: err=%v; want not_null_violation on application_id", err)
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
	_, err := repo.CreateEnrollment(ctx, models.ProgramEnrollmentInput{Program: models.ProgramCreateInput{ID: "00000000-0000-0000-0000-000000000061", CreatorUserID: "00000000-0000-0000-0000-000000000060", ProjectUID: &projectUID, Name: "Rollback", Slug: "rollback", Status: models.ProgramStatusPending}, Skills: []string{"Go", "Go"}})
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

func TestEnrollmentIntegration_PersistsProjectMetadataInIndexSnapshot(t *testing.T) {
	pool := integrationPool(t)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ('00000000-0000-0000-0000-000000000060', 'enroll-admin', 'Enroll Admin')`); err != nil {
		t.Fatal(err)
	}
	projectUID, projectSlug, projectName, projectLogo := "00000000-0000-0000-0000-000000000099", "enroll-project", "Enroll Project", "https://example.com/logo.svg"
	program, err := NewProgramRepository(pool).CreateEnrollment(ctx, models.ProgramEnrollmentInput{Program: models.ProgramCreateInput{ID: "00000000-0000-0000-0000-000000000061", CreatorUserID: "00000000-0000-0000-0000-000000000060", ProjectUID: &projectUID, ProjectSlug: &projectSlug, ProjectName: &projectName, ProjectLogoURL: &projectLogo, Name: "Enroll", Slug: "enroll", Status: models.ProgramStatusPending}, Skills: []string{"Go"}})
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}
	if program.ProjectSlug == nil || *program.ProjectSlug != projectSlug || program.ProjectName == nil || *program.ProjectName != projectName {
		t.Fatalf("returned project metadata = %v %v", program.ProjectSlug, program.ProjectName)
	}
	var data []byte
	if err := pool.QueryRow(ctx, `SELECT data FROM index_outbox WHERE object_type = 'mentorship_program' AND object_uid = $1`, program.ID).Scan(&data); err != nil {
		t.Fatalf("read program index snapshot: %v", err)
	}
	var document struct {
		ProjectSlug    string   `json:"project_slug"`
		ProjectName    string   `json:"project_name"`
		ProjectLogoURL string   `json:"project_logo_url"`
		Skills         []string `json:"skills"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode program index snapshot: %v", err)
	}
	if document.ProjectSlug != projectSlug || document.ProjectName != projectName || document.ProjectLogoURL != projectLogo {
		t.Fatalf("program index project metadata = %+v", document)
	}
	if len(document.Skills) != 1 || document.Skills[0] != "Go" {
		t.Fatalf("program index skills = %v; want [Go]", document.Skills)
	}
}

func TestProgramUpdateIntegration_ReplacesSkills(t *testing.T) {
	pool := integrationPool(t)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ('00000000-0000-0000-0000-000000000060', 'enroll-admin', 'Enroll Admin')`); err != nil {
		t.Fatal(err)
	}
	repo := NewProgramRepository(pool)
	projectUID := "00000000-0000-0000-0000-000000000099"
	program, err := repo.CreateEnrollment(ctx, models.ProgramEnrollmentInput{Program: models.ProgramCreateInput{ID: "00000000-0000-0000-0000-000000000061", CreatorUserID: "00000000-0000-0000-0000-000000000060", ProjectUID: &projectUID, Name: "Skills", Slug: "skills", Status: models.ProgramStatusPending}, Skills: []string{"Go", "Rust"}})
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}
	var keptID string
	if err := pool.QueryRow(ctx, `SELECT id FROM program_skills WHERE program_id = $1 AND skill = 'Go'`, program.ID).Scan(&keptID); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Update(ctx, program.ID, models.ProgramUpdateInput{Skills: []string{"Go", "Kubernetes"}}); err != nil {
		t.Fatalf("update skills: %v", err)
	}
	skills, err := repo.ListSkills(ctx, program.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, skill := range skills {
		got[skill.Skill] = skill.ID
	}
	if len(got) != 2 || got["Kubernetes"] == "" || got["Go"] != keptID {
		t.Fatalf("skills after update = %v; want Go (id %s) and Kubernetes", got, keptID)
	}
	var data []byte
	if err := pool.QueryRow(ctx, `SELECT data FROM index_outbox WHERE object_type = 'mentorship_program' AND object_uid = $1`, program.ID).Scan(&data); err != nil {
		t.Fatalf("read program index snapshot: %v", err)
	}
	var document struct {
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode program index snapshot: %v", err)
	}
	if len(document.Skills) != 2 || document.Skills[0] != "Go" || document.Skills[1] != "Kubernetes" {
		t.Fatalf("program index skills = %v; want [Go Kubernetes]", document.Skills)
	}

	name := "Skills Renamed"
	if _, err := repo.Update(ctx, program.ID, models.ProgramUpdateInput{Name: &name}); err != nil {
		t.Fatalf("update name: %v", err)
	}
	if skills, err := repo.ListSkills(ctx, program.ID); err != nil || len(skills) != 2 {
		t.Fatalf("skills after name-only update = %d, %v; want 2 unchanged", len(skills), err)
	}
}

func TestProgramUpdateIntegration_ReplacesOpenTerms(t *testing.T) {
	pool := integrationPool(t)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	const userID = "00000000-0000-0000-0000-000000000070"
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ($1, 'terms-admin', 'Terms Admin')`, userID); err != nil {
		t.Fatal(err)
	}
	appStart := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)
	appEnd, start, end := appStart.Add(24*time.Hour), appStart.Add(48*time.Hour), appStart.Add(30*24*time.Hour)
	term := func(id, name string) models.ProgramOpenTermInput {
		return models.ProgramOpenTermInput{ID: id, Name: name, ApplicationStartDate: &appStart, ApplicationEndDate: &appEnd, StartDateTime: &start, EndDateTime: &end}
	}
	const keptID, removedID, closedID = "00000000-0000-0000-0000-000000000072", "00000000-0000-0000-0000-000000000073", "00000000-0000-0000-0000-000000000074"
	repo := NewProgramRepository(pool)
	projectUID := "00000000-0000-0000-0000-000000000099"
	program, err := repo.CreateEnrollment(ctx, models.ProgramEnrollmentInput{
		Program: models.ProgramCreateInput{ID: "00000000-0000-0000-0000-000000000071", CreatorUserID: userID, ProjectUID: &projectUID, Name: "Terms", Slug: "terms", Status: models.ProgramStatusPending},
		Terms: []models.ProgramTermCreateInput{
			{ID: keptID, Name: "Fall", ApplicationStartDate: &appStart, ApplicationEndDate: &appEnd, StartDateTime: &start, EndDateTime: &end},
			{ID: removedID, Name: "Winter", ApplicationStartDate: &appStart, ApplicationEndDate: &appEnd, StartDateTime: &start, EndDateTime: &end},
		},
		Skills: []string{"Go"},
	})
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO program_terms (id, program_id, name, status) VALUES ($1, $2, 'Spring 2025', 'closed')`, closedID, program.ID); err != nil {
		t.Fatal(err)
	}
	termsByName := func() map[string][2]string {
		rows, err := pool.Query(ctx, `SELECT id::text, name, status FROM program_terms WHERE program_id = $1`, program.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		got := map[string][2]string{}
		for rows.Next() {
			var id, name, status string
			if err := rows.Scan(&id, &name, &status); err != nil {
				t.Fatal(err)
			}
			got[name] = [2]string{id, status}
		}
		return got
	}

	if _, err := repo.Update(ctx, program.ID, models.ProgramUpdateInput{Terms: []models.ProgramOpenTermInput{term(keptID, "Fall 2026"), term("", "Spring 2027")}}); err != nil {
		t.Fatalf("replace open terms: %v", err)
	}
	got := termsByName()
	if got["Fall 2026"] != [2]string{keptID, "open"} || got["Spring 2027"][1] != "open" || got["Winter"] != [2]string{removedID, "deleted"} || got["Spring 2025"] != [2]string{closedID, "closed"} {
		t.Fatalf("terms after replace = %v; want Fall 2026 kept, Spring 2027 added, Winter deleted, Spring 2025 closed and untouched", got)
	}
	newID := got["Spring 2027"][0]

	// Only open terms of the program can be listed.
	if _, err := repo.Update(ctx, program.ID, models.ProgramUpdateInput{Terms: []models.ProgramOpenTermInput{term(keptID, "Fall 2026"), term(newID, "Spring 2027"), term(closedID, "Edited")}}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("listing a closed term: got %v; want ErrInvalidInput", err)
	}

	// An open term with applications cannot be removed.
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ('00000000-0000-0000-0000-000000000075', $1, $2, 'mentee', 'pending')`, keptID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Update(ctx, program.ID, models.ProgramUpdateInput{Terms: []models.ProgramOpenTermInput{term(newID, "Spring 2027")}}); !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("removing a term with applications: got %v; want ErrStateLocked", err)
	}
	if after := termsByName(); after["Fall 2026"] != [2]string{keptID, "open"} || after["Spring 2025"] != [2]string{closedID, "closed"} {
		t.Fatalf("terms after rejected updates = %v; want unchanged", after)
	}
}

func TestProgramUpdateIntegration_ChangesProject(t *testing.T) {
	pool := integrationPool(t)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ('00000000-0000-0000-0000-000000000060', 'enroll-admin', 'Enroll Admin')`); err != nil {
		t.Fatal(err)
	}
	repo := NewProgramRepository(pool)
	oldUID, oldSlug, oldName, oldLogo := "00000000-0000-0000-0000-000000000099", "old-project", "Old Project", "https://example.com/old.svg"
	program, err := repo.CreateEnrollment(ctx, models.ProgramEnrollmentInput{Program: models.ProgramCreateInput{ID: "00000000-0000-0000-0000-000000000061", CreatorUserID: "00000000-0000-0000-0000-000000000060", ProjectUID: &oldUID, ProjectSlug: &oldSlug, ProjectName: &oldName, ProjectLogoURL: &oldLogo, Name: "Moving", Slug: "moving", Status: models.ProgramStatusPending}, Skills: []string{"Go"}})
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}

	newUID, newSlug, newName := "00000000-0000-0000-0000-000000000098", "new-project", "New Project"
	updated, err := repo.Update(ctx, program.ID, models.ProgramUpdateInput{ProjectUID: &newUID, ProjectSlug: &newSlug, ProjectName: &newName})
	if err != nil {
		t.Fatalf("update project: %v", err)
	}
	if updated.ProjectUID == nil || *updated.ProjectUID != newUID || updated.ProjectSlug == nil || *updated.ProjectSlug != newSlug ||
		updated.ProjectName == nil || *updated.ProjectName != newName {
		t.Fatalf("project after update = %v %v %v", updated.ProjectUID, updated.ProjectSlug, updated.ProjectName)
	}
	if updated.ProjectLogoURL != nil {
		t.Fatalf("project_logo_url = %q; want old logo cleared", *updated.ProjectLogoURL)
	}

	name := "Moving Renamed"
	if updated, err = repo.Update(ctx, program.ID, models.ProgramUpdateInput{Name: &name}); err != nil {
		t.Fatalf("update name: %v", err)
	}
	if updated.ProjectUID == nil || *updated.ProjectUID != newUID {
		t.Fatalf("project_uid after name-only update = %v; want %s", updated.ProjectUID, newUID)
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
	if deadLettered, err := repo.DeadLetter(ctx, markers[0], "permanent builder failure"); err != nil || !deadLettered {
		t.Fatalf("dead-letter: dead_lettered=%v err=%v", deadLettered, err)
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

func TestFGAOutboxIntegration_DeadLetterYieldsToNewerGeneration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-racing", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-racing", "update_access"); err != nil {
		t.Fatalf("enqueue newer generation: %v", err)
	}
	if deadLettered, err := repo.DeadLetter(ctx, markers[0], "stale failure"); err != nil || deadLettered {
		t.Fatalf("dead-letter over newer generation: dead_lettered=%v err=%v; want false", deadLettered, err)
	}
	if retried, err := repo.Retry(ctx, markers[0], time.Now(), "stale failure"); err != nil || !retried {
		t.Fatalf("retry newer generation: retried=%v err=%v; want true", retried, err)
	}
	if retried, err := repo.Retry(ctx, markers[0], time.Now(), "stale failure"); err != nil || retried {
		t.Fatalf("retry after release: retried=%v err=%v; want false", retried, err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if state != "pending" {
		t.Fatalf("state=%q; want pending", state)
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
	if deadLettered, err := repo.DeadLetter(ctx, markers[0], "dependency unavailable"); err != nil || !deadLettered {
		t.Fatalf("dead-letter: dead_lettered=%v err=%v", deadLettered, err)
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

func TestProgramMemberIntegration_SelfRequestListAndLookup(t *testing.T) {
	pool := integrationPool(t)
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	repo := NewProgramMemberRepository(pool)

	requested := models.ProgramMemberStatusRequested
	if _, err := repo.Create(ctx, fixture.ProgramID, models.ProgramMemberCreateInput{
		ID:         "00000000-0000-0000-0000-000000000022",
		UserID:     fixture.UserID,
		MemberType: models.MemberTypeMentor,
		Status:     &requested,
	}); err != nil {
		t.Fatalf("create requested mentor row: %v", err)
	}

	mentor, err := repo.FindByProgramUserAndType(ctx, fixture.ProgramID, fixture.UserID, models.MemberTypeMentor)
	if err != nil {
		t.Fatalf("find mentor row: %v", err)
	}
	if mentor.ID != "00000000-0000-0000-0000-000000000022" || mentor.Status == nil || *mentor.Status != requested {
		t.Fatalf("mentor row = %+v; want the requested row", mentor)
	}
	if _, err := repo.FindByProgramUserAndType(ctx, fixture.ProgramID, "00000000-0000-0000-0000-000000000099", models.MemberTypeMentor); !errors.Is(err, domain.ErrProgramMemberNotFound) {
		t.Fatalf("find for another user: got %v; want ErrProgramMemberNotFound", err)
	}

	all, meta, err := repo.ListByUser(ctx, fixture.UserID, models.ProgramMemberFilter{})
	if err != nil {
		t.Fatalf("list all memberships: %v", err)
	}
	if meta.Total != 2 || len(all) != 2 {
		t.Fatalf("got %d memberships (total %d); want the program_admin and mentor rows", len(all), meta.Total)
	}

	mentors, meta, err := repo.ListByUser(ctx, fixture.UserID, models.ProgramMemberFilter{MemberType: string(models.MemberTypeMentor)})
	if err != nil {
		t.Fatalf("list mentor memberships: %v", err)
	}
	if meta.Total != 1 || len(mentors) != 1 || mentors[0].ProgramName != "Fixture Program" || mentors[0].Status == nil || *mentors[0].Status != requested {
		t.Fatalf("mentor memberships = %+v (total %d); want one requested row for Fixture Program", mentors, meta.Total)
	}

	// The transition applies only from the expected status.
	if _, err := repo.UpdateIfStatus(ctx, mentor.ID, []models.ProgramMemberStatus{models.ProgramMemberStatusWithdrawn}, models.ProgramMemberUpdateInput{Status: &requested}); !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("transition from withdrawn on a requested row: got %v; want ErrInvalidStateTransition", err)
	}
	withdrawnStatus := models.ProgramMemberStatusWithdrawn
	withdrawn, err := repo.UpdateIfStatus(ctx, mentor.ID, []models.ProgramMemberStatus{models.ProgramMemberStatusRequested}, models.ProgramMemberUpdateInput{Status: &withdrawnStatus})
	if err != nil {
		t.Fatalf("withdraw requested row: %v", err)
	}
	if withdrawn.Status == nil || *withdrawn.Status != models.ProgramMemberStatusWithdrawn {
		t.Fatalf("status = %v; want withdrawn", withdrawn.Status)
	}

	// Neither a requested nor a withdrawn row grants anything, so neither may
	// reach the FGA outbox.
	var markers int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM fga_outbox WHERE marker_kind = 'membership' AND relation = 'mentor'`).Scan(&markers); err != nil {
		t.Fatal(err)
	}
	if markers != 0 {
		t.Fatalf("self-service mentor row enqueued %d FGA mentor markers; want 0", markers)
	}
}
