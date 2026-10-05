// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

const (
	fileTestCDN         = "https://cdn.example.org/mentorship"
	fileTestApplication = "00000000-0000-0000-0000-000000000080"
	fileTestTask        = "00000000-0000-0000-0000-000000000081"
	fileTestProfile     = "00000000-0000-0000-0000-000000000082"
)

var fileTestBuckets = []domain.ObjectBucket{domain.ObjectBucketLogos, domain.ObjectBucketAttachments}

func fileIntegrationPool(t *testing.T) (*pgxpool.Pool, integrationFixture) {
	t.Helper()
	pool := integrationPool(t)
	if _, err := pool.Exec(context.Background(), `TRUNCATE object_deletions`); err != nil {
		t.Fatalf("reset object_deletions: %v", err)
	}
	fixture := seedIntegrationFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, 'mentee', 'accepted')`, fileTestApplication, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status) VALUES ($1, $2, $3, $4, 'in_progress')`, fileTestTask, fileTestApplication, fixture.OpenTerm, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_profiles (id, user_id, profile_type) VALUES ($1, $2, 'mentor')`, fileTestProfile, fixture.UserID); err != nil {
		t.Fatal(err)
	}
	return pool, fixture
}

type deletionRow struct {
	bucket, locator, state string
}

func deletionRows(t *testing.T, pool *pgxpool.Pool) []deletionRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT bucket, locator, state FROM object_deletions ORDER BY created_on, locator`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []deletionRow
	for rows.Next() {
		var r deletionRow
		if err := rows.Scan(&r.bucket, &r.locator, &r.state); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestFileRepositoryIntegration_ReplaceCancelsGraceEntryAndQueuesPrevious(t *testing.T) {
	pool, _ := fileIntegrationPool(t)
	ctx := context.Background()
	queue, files := NewObjectDeletionRepository(pool), NewFileRepository(pool)

	first, err := queue.Schedule(ctx, domain.ObjectBucketAttachments, "key-1", time.Hour)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if err := files.ReplaceTaskFile(ctx, domain.FileReplacement{RowID: fileTestTask, Next: "key-1", PendingDeletionID: first}); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	second, _ := queue.Schedule(ctx, domain.ObjectBucketAttachments, "key-2", time.Hour)
	if err := files.ReplaceTaskFile(ctx, domain.FileReplacement{RowID: fileTestTask, Previous: ptrTo("key-1"), Next: "key-2", PendingDeletionID: second}); err != nil {
		t.Fatalf("second replace: %v", err)
	}
	if got := deletionRows(t, pool); len(got) != 1 || got[0] != (deletionRow{"attachments", "key-1", "pending"}) {
		t.Fatalf("queue = %+v; want only the superseded key", got)
	}

	// A replace that read a stale value loses, and its own grace entry stays to reclaim the object.
	third, _ := queue.Schedule(ctx, domain.ObjectBucketAttachments, "key-3", time.Hour)
	err = files.ReplaceTaskFile(ctx, domain.FileReplacement{RowID: fileTestTask, Previous: ptrTo("key-1"), Next: "key-3", PendingDeletionID: third})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale replace err = %v; want ErrConflict", err)
	}
	if len(deletionRows(t, pool)) != 2 {
		t.Fatalf("queue = %+v; the losing key must stay queued", deletionRows(t, pool))
	}
}

func TestFileRepositoryIntegration_ReplaceFailsOnceRelayClaimedTheNewKey(t *testing.T) {
	pool, _ := fileIntegrationPool(t)
	ctx := context.Background()
	queue, files := NewObjectDeletionRepository(pool), NewFileRepository(pool)

	pending, _ := queue.Schedule(ctx, domain.ObjectBucketAttachments, "late-key", 0)
	if claimed, err := queue.Claim(ctx, fileTestBuckets, 10); err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	err := files.ReplaceTaskFile(ctx, domain.FileReplacement{RowID: fileTestTask, Next: "late-key", PendingDeletionID: pending})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v; want ErrConflict", err)
	}
	var file *string
	if err := pool.QueryRow(ctx, `SELECT file FROM tasks WHERE id = $1`, fileTestTask).Scan(&file); err != nil || file != nil {
		t.Fatalf("task file = %v (err %v); the row must not point at bytes being deleted", file, err)
	}
}

func TestFileRepositoryIntegration_TaskFileStateRules(t *testing.T) {
	pool, _ := fileIntegrationPool(t)
	ctx := context.Background()
	files := NewFileRepository(pool)
	if _, err := pool.Exec(ctx, `UPDATE tasks SET file = 'key', status = 'submitted' WHERE id = $1`, fileTestTask); err != nil {
		t.Fatal(err)
	}
	if err := files.ClearTaskFile(ctx, fileTestTask, "key"); !errors.Is(err, domain.ErrStateLocked) {
		t.Fatalf("clear submitted err = %v; want ErrStateLocked", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tasks SET status = 'in_progress' WHERE id = $1`, fileTestTask); err != nil {
		t.Fatal(err)
	}
	if err := files.ClearTaskFile(ctx, fileTestTask, "key"); err != nil {
		t.Fatalf("clear in-progress: %v", err)
	}
	if got := deletionRows(t, pool); len(got) != 1 || got[0].locator != "key" {
		t.Fatalf("queue = %+v", got)
	}
}

func TestFileRepositoryIntegration_ProfileLogoKeepsAvatarAliased(t *testing.T) {
	pool, fixture := fileIntegrationPool(t)
	ctx := context.Background()
	queue, files := NewObjectDeletionRepository(pool), NewFileRepository(pool)
	logo := fileTestCDN + "/abc-logo.png"

	pending, _ := queue.Schedule(ctx, domain.ObjectBucketLogos, logo, time.Hour)
	if err := files.ReplaceProfileLogo(ctx, domain.FileReplacement{RowID: fileTestProfile, Next: logo, PendingDeletionID: pending}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	// Login must not overwrite the alias with the OIDC picture.
	lfid, picture := "fixture-user", "https://auth0.example/picture.png"
	if _, err := NewUserRepository(pool).UpsertByLFID(ctx, models.UserCreateInput{LFID: &lfid, AvatarURL: &picture}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	assertAvatar(t, pool, fixture.UserID, &logo)

	if err := NewUserProfileRepository(pool).Delete(ctx, fileTestProfile); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	assertAvatar(t, pool, fixture.UserID, nil)
	if got := deletionRows(t, pool); len(got) != 1 || got[0] != (deletionRow{"logos", logo, "pending"}) {
		t.Fatalf("queue = %+v", got)
	}
}

func TestFileRepositoryIntegration_ParentDeletesQueueHeldFiles(t *testing.T) {
	pool, fixture := fileIntegrationPool(t)
	ctx := context.Background()
	logo := fileTestCDN + "/abc-program.png"
	if _, err := pool.Exec(ctx, `UPDATE programs SET logo_url = $2 WHERE id = $1`, fixture.ProgramID, logo); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tasks SET file = 'task-key' WHERE id = $1`, fileTestTask); err != nil {
		t.Fatal(err)
	}
	if err := NewProgramRepository(pool).Delete(ctx, fixture.ProgramID); err != nil {
		t.Fatalf("delete program: %v", err)
	}
	got := deletionRows(t, pool)
	want := map[deletionRow]bool{{"logos", logo, "pending"}: true, {"attachments", "task-key", "pending"}: true}
	if len(got) != len(want) || !want[got[0]] || !want[got[1]] {
		t.Fatalf("queue = %+v; want the program logo and the cascaded task file", got)
	}
}

func TestObjectDeletionIntegration_ClaimReferenceRetryAndDeadLetter(t *testing.T) {
	pool, fixture := fileIntegrationPool(t)
	ctx := context.Background()
	queue := NewObjectDeletionRepository(pool)
	queue.maxAttempts = 2
	queue.retryDelay = 0
	shared := fileTestCDN + "/shared.png"
	if _, err := pool.Exec(ctx, `UPDATE programs SET logo_url = $2 WHERE id = $1`, fixture.ProgramID, shared); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Schedule(ctx, domain.ObjectBucketLogos, shared, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Schedule(ctx, domain.ObjectBucketLogos, fileTestCDN+"/future.png", time.Hour); err != nil {
		t.Fatal(err)
	}

	claimed, err := queue.Claim(ctx, fileTestBuckets, 10)
	if err != nil || len(claimed) != 1 || claimed[0].Locator != shared {
		t.Fatalf("claim = %+v, %v; want only the due entry", claimed, err)
	}
	if referenced, err := queue.IsReferenced(ctx, shared); err != nil || !referenced {
		t.Fatalf("referenced = %v, %v; want true", referenced, err)
	}
	if dead, err := queue.MarkRetry(ctx, claimed[0], errors.New("s3 down")); err != nil || dead {
		t.Fatalf("first retry dead = %v, %v", dead, err)
	}
	claimed, _ = queue.Claim(ctx, fileTestBuckets, 10)
	if dead, err := queue.MarkRetry(ctx, claimed[0], errors.New("s3 down")); err != nil || !dead {
		t.Fatalf("second retry dead = %v, %v; want dead-lettered", dead, err)
	}
	if claimed, _ := queue.Claim(ctx, fileTestBuckets, 10); len(claimed) != 0 {
		t.Fatalf("dead-lettered entry was reclaimed: %+v", claimed)
	}
}

func TestObjectDeletionIntegration_MarkDoneRemovesTheEntry(t *testing.T) {
	pool, _ := fileIntegrationPool(t)
	ctx := context.Background()
	queue := NewObjectDeletionRepository(pool)
	if _, err := queue.Schedule(ctx, domain.ObjectBucketAttachments, "done-key", 0); err != nil {
		t.Fatal(err)
	}
	claimed, err := queue.Claim(ctx, fileTestBuckets, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %+v, %v", claimed, err)
	}
	if err := queue.MarkDone(ctx, claimed[0]); err != nil {
		t.Fatalf("mark done: %v", err)
	}
	if got := deletionRows(t, pool); len(got) != 0 {
		t.Fatalf("queue = %+v; a completed entry must not be retained", got)
	}
}

func TestObjectDeletionIntegration_ClaimSkipsUnconfiguredBuckets(t *testing.T) {
	pool, _ := fileIntegrationPool(t)
	ctx := context.Background()
	queue := NewObjectDeletionRepository(pool)
	if _, err := queue.Schedule(ctx, domain.ObjectBucketAttachments, "task-key", 0); err != nil {
		t.Fatal(err)
	}
	if claimed, err := queue.Claim(ctx, []domain.ObjectBucket{domain.ObjectBucketLogos}, 10); err != nil || len(claimed) != 0 {
		t.Fatalf("claim = %+v, %v; an attachments entry must wait for its bucket", claimed, err)
	}
	if got := deletionRows(t, pool); len(got) != 1 || got[0].state != "pending" {
		t.Fatalf("queue = %+v", got)
	}
}

// A file write that commits while a parent delete waits on its lock must be queued by that delete.
func TestFileRepositoryIntegration_ParentDeletesQueueFilesWrittenConcurrently(t *testing.T) {
	tests := map[string]func(pool *pgxpool.Pool, fixture integrationFixture) error{
		"task": func(pool *pgxpool.Pool, _ integrationFixture) error {
			return NewTaskRepository(pool).Delete(context.Background(), fileTestTask)
		},
		"application": func(pool *pgxpool.Pool, _ integrationFixture) error {
			return NewApplicationRepository(pool).Delete(context.Background(), fileTestApplication)
		},
		"program": func(pool *pgxpool.Pool, fixture integrationFixture) error {
			return NewProgramRepository(pool).Delete(context.Background(), fixture.ProgramID)
		},
	}
	for name, deleteParent := range tests {
		t.Run(name, func(t *testing.T) {
			pool, fixture := fileIntegrationPool(t)
			ctx := context.Background()
			writer, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = writer.Rollback(ctx) }()
			if _, err := writer.Exec(ctx, `UPDATE tasks SET file = 'racing-key' WHERE id = $1`, fileTestTask); err != nil {
				t.Fatal(err)
			}

			done := make(chan error, 1)
			go func() { done <- deleteParent(pool, fixture) }()
			select {
			case err := <-done:
				t.Fatalf("delete finished while the file write held the task: %v", err)
			case <-time.After(200 * time.Millisecond):
			}
			if err := writer.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("delete: %v", err)
			}
			if got := deletionRows(t, pool); len(got) != 1 || got[0].locator != "racing-key" {
				t.Fatalf("queue = %+v; want the concurrently written key", got)
			}
		})
	}
}

func TestTaskRepositoryIntegration_SubmitRequiresFileAtomically(t *testing.T) {
	pool, _ := fileIntegrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE tasks SET submit_file = 'required' WHERE id = $1`, fileTestTask); err != nil {
		t.Fatal(err)
	}
	submitted := models.TaskStatusSubmitted
	_, err := NewTaskRepository(pool).Update(ctx, fileTestTask, models.TaskUpdateInput{Status: &submitted})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v; want ErrInvalidInput", err)
	}
	if _, err := NewTaskRepository(pool).Update(ctx, "00000000-0000-0000-0000-0000000000ff", models.TaskUpdateInput{Status: &submitted}); !errors.Is(err, domain.ErrTaskNotFound) {
		t.Fatalf("missing task err = %v; want ErrTaskNotFound", err)
	}
}

func TestUserProfileRepositoryIntegration_RedactsResumeLink(t *testing.T) {
	pool, _ := fileIntegrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE user_profiles SET profile_links = '{"githubProfileLink":"https://github.com/x","resumeLink":"https://x/cv.pdf"}' WHERE id = $1`, fileTestProfile); err != nil {
		t.Fatal(err)
	}
	p, err := NewUserProfileRepository(pool).GetByID(ctx, fileTestProfile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(p.ProfileLinks), "resumeLink") || !strings.Contains(string(p.ProfileLinks), "githubProfileLink") {
		t.Fatalf("profile_links = %s; want resumeLink removed and other links kept", p.ProfileLinks)
	}
}

func assertAvatar(t *testing.T, pool *pgxpool.Pool, userID string, want *string) {
	t.Helper()
	var got *string
	if err := pool.QueryRow(context.Background(), `SELECT avatar_url FROM users WHERE id = $1`, userID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if (got == nil) != (want == nil) || (got != nil && *got != *want) {
		t.Fatalf("avatar_url = %v; want %v", got, want)
	}
}

func ptrTo(s string) *string { return &s }
