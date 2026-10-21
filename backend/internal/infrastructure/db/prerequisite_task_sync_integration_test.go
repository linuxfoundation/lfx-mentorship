// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// syncSeeder inserts applications and their tasks for the prerequisite sync.
type syncSeeder struct {
	t    *testing.T
	pool *pgxpool.Pool
	seq  int
}

func (s *syncSeeder) apply(termID, role, status string) string {
	s.t.Helper()
	s.seq++
	userID := fmt.Sprintf("00000000-0000-0000-0001-%012d", s.seq)
	appID := fmt.Sprintf("00000000-0000-0000-0002-%012d", s.seq)
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, `INSERT INTO users (id, lfid, name) VALUES ($1, $2, 'Applicant')`, userID, fmt.Sprintf("sync-applicant-%d", s.seq)); err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO applications (id, program_term_id, user_id, role, status) VALUES ($1, $2, $3, $4, $5)`,
		appID, termID, userID, role, status); err != nil {
		s.t.Fatal(err)
	}
	return appID
}

func (s *syncSeeder) task(appID, name, category, status string) string {
	s.t.Helper()
	s.seq++
	id := fmt.Sprintf("00000000-0000-0000-0003-%012d", s.seq)
	if _, err := s.pool.Exec(context.Background(), `
		INSERT INTO tasks (id, application_id, program_term_id, assignee_id, name, description, category, status, due_date, created_by)
		SELECT $1, a.id, a.program_term_id, a.user_id, $2, 'Old description', $3, $4, '2020-01-01', a.user_id::text FROM applications a WHERE a.id = $5`,
		id, name, category, status, appID); err != nil {
		s.t.Fatal(err)
	}
	return id
}

// createdBy records who created a task, and its owner; a seeded task is otherwise a clone made
// when its applicant applied.
func (s *syncSeeder) createdBy(taskID, creator string, ownerID *string) {
	s.t.Helper()
	if _, err := s.pool.Exec(context.Background(), `UPDATE tasks SET created_by = $2, owner_id = $3 WHERE id = $1`, taskID, creator, ownerID); err != nil {
		s.t.Fatal(err)
	}
}

type syncedTask struct {
	Name, Status    string
	Description     *string
	SubmitFile, Due *string
	Custom          bool
}

func syncedTasks(t *testing.T, pool *pgxpool.Pool, appID string) map[string]syncedTask {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT name, status, description, submit_file, due_date, COALESCE(custom, false)
		FROM tasks WHERE application_id = $1`, appID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]syncedTask{}
	for rows.Next() {
		var task syncedTask
		if err := rows.Scan(&task.Name, &task.Status, &task.Description, &task.SubmitFile, &task.Due, &task.Custom); err != nil {
			t.Fatal(err)
		}
		got[task.Name] = task
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func applyChanges(t *testing.T, pool *pgxpool.Pool, programID string, changes models.PrerequisiteChanges) []string {
	t.Helper()
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	_, flagged, err := NewProgramRepository(pool).Update(ctx, programID, models.ProgramUpdateInput{}, changes)
	if err != nil {
		t.Fatalf("update program: %v", err)
	}
	return flagged
}

func TestProgramRepositoryIntegration_UpdateAppliesPrerequisiteChanges(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}

	pending := s.apply(f.OpenTerm, "mentee", "pending")
	s.task(pending, "Resume", "prerequisite", "incomplete")
	essay := s.task(pending, "Essay", "prerequisite", "in_progress")
	s.task(pending, "Video", "prerequisite", "submitted")
	uploaded := s.task(pending, "Notes", "prerequisite", "in_progress")
	if _, err := pool.Exec(context.Background(), `UPDATE tasks SET file = 'attachments/notes.pdf' WHERE id = $1`, uploaded); err != nil {
		t.Fatal(err)
	}
	s.task(pending, "Portfolio", "prerequisite", "incomplete")
	// Tasks a reviewer or a service created are never changed, and their names still count as
	// present. A legacy clone, created as the program's creator, is changed.
	reviewer := f.UserID
	s.createdBy(s.task(pending, "Brief", "prerequisite", "incomplete"), reviewer, &reviewer)
	s.createdBy(s.task(pending, "Bot task", "prerequisite", "incomplete"), "00000000-0000-0000-0000-0000000000aa", nil)
	s.createdBy(s.task(pending, "Mentor task", "prerequisite", "complete"), reviewer, &reviewer)
	if _, err := pool.Exec(context.Background(), `UPDATE programs SET lfid = 'legacy-creator' WHERE id = $1`, f.ProgramID); err != nil {
		t.Fatal(err)
	}
	s.createdBy(s.task(pending, "Legacy", "prerequisite", "incomplete"), "legacy-creator", &reviewer)
	// An edit reaches a task the mentee uploaded a file for; only removal keeps it.
	cv := s.task(pending, "CV", "prerequisite", "in_progress")
	if _, err := pool.Exec(context.Background(), `UPDATE tasks SET file = 'attachments/cv.pdf' WHERE id = $1`, cv); err != nil {
		t.Fatal(err)
	}
	s.task(pending, "Intro call", "non_prerequisite", "incomplete")
	// A pending application on a closed term takes the changes too.
	closedTerm := s.apply(f.ClosedTerm, "mentee", "pending")
	s.task(closedTerm, "Resume", "prerequisite", "complete")
	accepted := s.apply(f.OpenTerm, "mentee", "accepted")
	s.task(accepted, "Essay", "prerequisite", "incomplete")
	mentor := s.apply(f.OpenTerm, "mentor", "pending")
	s.task(mentor, "Essay", "prerequisite", "incomplete")
	const otherProgram = "00000000-0000-0000-0000-000000000798"
	if _, err := pool.Exec(context.Background(), `INSERT INTO programs (id, lf_project_uid, name, slug, status) VALUES ($1, '00000000-0000-0000-0000-000000000097', 'Other Program', 'other-program', 'published')`, otherProgram); err != nil {
		t.Fatal(err)
	}
	otherTerm := "00000000-0000-0000-0000-000000000796"
	if _, err := pool.Exec(context.Background(), `INSERT INTO program_terms (id, program_id, name, status) VALUES ($1, $2, 'Other', 'open')`, otherTerm, otherProgram); err != nil {
		t.Fatal(err)
	}
	other := s.apply(otherTerm, "mentee", "pending")
	s.task(other, "Essay", "prerequisite", "incomplete")

	description, required, due := "Upload a PDF", "required", "2030-01-31"
	applyChanges(t, pool, f.ProgramID, models.PrerequisiteChanges{
		Removed: []string{"essay", "video", "notes", "brief", "legacy"},
		Edited: []models.PrerequisiteTemplate{
			{Name: "resume", Description: &description, SubmitFile: &required, DueDate: &due},
			{Name: "CV", DueDate: &due},
			{Name: "Bot task", DueDate: &due},
		},
		Added: []models.PrerequisiteTemplate{{Name: "Cover letter", Custom: true}, {Name: "mentor task"}},
	})

	got := syncedTasks(t, pool, pending)
	want := map[string]string{
		"resume":       "incomplete",
		"Video":        "submitted",   // submitted work is kept
		"Notes":        "in_progress", // so is an uploaded file
		"Portfolio":    "incomplete",  // a prerequisite the edit did not touch
		"Brief":        "incomplete",  // a reviewer created it
		"Bot task":     "incomplete",  // a service created it
		"Mentor task":  "complete",    // not added again under the same name
		"CV":           "in_progress",
		"Intro call":   "incomplete",
		"Cover letter": "incomplete",
	}
	if len(got) != len(want) {
		t.Fatalf("pending application tasks = %v; want %v", keys(got), want)
	}
	for name, status := range want {
		if g, ok := got[name]; !ok || g.Status != status {
			t.Errorf("task %q = %+v; want status %s", name, g, status)
		}
	}
	if resume := got["resume"]; deref(resume.Description) != description || deref(resume.SubmitFile) != required || deref(resume.Due) != due {
		t.Errorf("unsubmitted task kept old content: %+v", resume)
	}
	if deref(got["CV"].Due) != due {
		t.Errorf("edited task with an uploaded file kept its old due date: %+v", got["CV"])
	}
	if !got["Cover letter"].Custom {
		t.Errorf("added task lost its custom flag: %+v", got["Cover letter"])
	}
	for _, name := range []string{"Video", "Portfolio", "Intro call", "Brief", "Bot task", "Mentor task"} {
		if deref(got[name].Description) != "Old description" {
			t.Errorf("task %q was changed: %+v", name, got[name])
		}
	}

	closed := syncedTasks(t, pool, closedTerm)
	if r := closed["Resume"]; r.Status != "complete" || deref(r.Description) != "Old description" {
		t.Errorf("complete task was changed: %+v", r)
	}
	if _, ok := closed["Cover letter"]; !ok {
		t.Errorf("closed-term pending application did not get the added prerequisite; tasks = %v", keys(closed))
	}
	for name, app := range map[string]string{"accepted": accepted, "mentor": mentor, "other program's": other} {
		if got := syncedTasks(t, pool, app); len(got) != 1 || deref(got["Essay"].Description) != "Old description" {
			t.Errorf("%s application's tasks were changed: %v", name, keys(got))
		}
	}

	// The deleted task leaves the search index and loses its access.
	var indexAction, fgaOperation string
	if err := pool.QueryRow(context.Background(), `SELECT action FROM index_outbox WHERE object_uid = $1 ORDER BY created_on DESC LIMIT 1`, essay).Scan(&indexAction); err != nil {
		t.Fatalf("index outbox for deleted task: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT desired_operation FROM fga_outbox WHERE object_uid = $1`, essay).Scan(&fgaOperation); err != nil {
		t.Fatalf("FGA outbox for deleted task: %v", err)
	}
	if indexAction != "deleted" || fgaOperation != "delete_access" {
		t.Errorf("deleted task index/FGA = %s/%s; want deleted/delete_access", indexAction, fgaOperation)
	}
}

func TestProgramRepositoryIntegration_UpdateFlagsApplicationCompletedByRemoval(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}

	app := s.apply(f.OpenTerm, "mentee", "pending")
	s.task(app, "Resume", "prerequisite", "submitted")
	s.task(app, "Essay", "prerequisite", "incomplete")
	outstanding := s.apply(f.OpenTerm, "mentee", "pending")
	s.task(outstanding, "Resume", "prerequisite", "incomplete")
	s.task(outstanding, "Essay", "prerequisite", "incomplete")
	sole := s.apply(f.OpenTerm, "mentee", "pending")
	s.task(sole, "Essay", "prerequisite", "incomplete")
	none := s.apply(f.OpenTerm, "mentee", "pending")

	before := programIndexGeneration(t, pool, f.ProgramID)
	flagged := applyChanges(t, pool, f.ProgramID, models.PrerequisiteChanges{Removed: []string{"essay"}})
	// The program's search document is refreshed once for the update, not once per application.
	if got := programIndexGeneration(t, pool, f.ProgramID) - before; got != 1 {
		t.Errorf("program index refreshes = %d; want 1", got)
	}

	slices.Sort(flagged)
	want := []string{app, sole}
	slices.Sort(want)
	if !slices.Equal(flagged, want) {
		t.Fatalf("flagged = %v; want %v, whose last outstanding prerequisite was removed (not %s or %s)", flagged, want, outstanding, none)
	}
	var submitted bool
	if err := pool.QueryRow(context.Background(), `SELECT tasks_submitted FROM applications WHERE id = $1`, app).Scan(&submitted); err != nil {
		t.Fatal(err)
	}
	if !submitted {
		t.Error("tasks_submitted = false; want true once the only outstanding prerequisite is removed")
	}
}

// A submission outside a program update still refreshes the program's search document itself.
func TestApplicationRepositoryIntegration_MarkTasksSubmittedRefreshesProgram(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}
	app := s.apply(f.OpenTerm, "mentee", "pending")
	s.task(app, "Essay", "prerequisite", "submitted")

	before := programIndexGeneration(t, pool, f.ProgramID)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	flipped, err := NewApplicationRepository(pool).MarkTasksSubmitted(ctx, app)
	if err != nil || !flipped {
		t.Fatalf("MarkTasksSubmitted = %v, %v; want true, nil", flipped, err)
	}
	if got := programIndexGeneration(t, pool, f.ProgramID) - before; got != 1 {
		t.Errorf("program index refreshes = %d; want 1", got)
	}
}

// programIndexGeneration counts the program's search document refreshes queued so far.
func programIndexGeneration(t *testing.T, pool *pgxpool.Pool, programID string) int64 {
	t.Helper()
	var generation int64
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(MAX(generation), 0) FROM index_outbox
		WHERE object_type = 'mentorship_program' AND object_uid = $1`, programID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	return generation
}

func TestProgramRepositoryIntegration_UpdateWithoutPrerequisiteChangesLeavesTasks(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}

	app := s.apply(f.OpenTerm, "mentee", "pending")
	s.task(app, "Essay", "prerequisite", "incomplete")

	name := "Renamed Program"
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	if _, _, err := NewProgramRepository(pool).Update(ctx, f.ProgramID, models.ProgramUpdateInput{Name: &name, TaskTemplates: json.RawMessage(`[]`)}, models.PrerequisiteChanges{}); err != nil {
		t.Fatal(err)
	}
	if got := syncedTasks(t, pool, app); len(got) != 1 {
		t.Errorf("tasks = %v; want the task kept when no prerequisite change is passed", keys(got))
	}
}

func keys(m map[string]syncedTask) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestProgramRepositoryIntegration_UpdateRejectsConcurrentTemplateChange(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}
	app := s.apply(f.OpenTerm, "mentee", "pending")
	s.task(app, "Essay", "prerequisite", "incomplete")
	if _, err := pool.Exec(context.Background(), `UPDATE programs SET task_templates = '[{"name":"Essay"},{"name":"X"}]' WHERE id = $1`, f.ProgramID); err != nil {
		t.Fatal(err)
	}

	// Diffed against a read from before X was added, so the save would drop X and keep its tasks.
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	_, _, err := NewProgramRepository(pool).Update(ctx, f.ProgramID, models.ProgramUpdateInput{
		TaskTemplates:         json.RawMessage(`[]`),
		ExpectedTaskTemplates: json.RawMessage(`[{"name":"Essay"}]`),
	}, models.PrerequisiteChanges{Removed: []string{"essay"}})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v; want ErrConflict", err)
	}
	if got := syncedTasks(t, pool, app); len(got) != 1 {
		t.Errorf("tasks = %v; want them kept when the update is refused", keys(got))
	}

	// Templates stored as JSONB compare by value, not by the formatting they were sent with.
	if _, _, err := NewProgramRepository(pool).Update(ctx, f.ProgramID, models.ProgramUpdateInput{
		TaskTemplates:         json.RawMessage(`[]`),
		ExpectedTaskTemplates: json.RawMessage(`[ {"name": "Essay"}, {"name": "X"} ]`),
	}, models.PrerequisiteChanges{Removed: []string{"essay", "x"}}); err != nil {
		t.Fatalf("update from the current templates: %v", err)
	}
	if got := syncedTasks(t, pool, app); len(got) != 0 {
		t.Errorf("tasks = %v; want the removed prerequisite's task deleted", keys(got))
	}
}

// Closing a term locks it and then its applications. A prerequisite change, whose task inserts
// key-share-lock the term, must take the terms first too, or the two can deadlock.
func TestProgramRepositoryIntegration_UpdateLocksTermsBeforeApplications(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}
	s.apply(f.OpenTerm, "mentee", "pending")

	ctx := context.Background()
	closing, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closing.Rollback(ctx) }()
	if _, err := closing.Exec(ctx, `SELECT 1 FROM program_terms WHERE id = $1 FOR UPDATE`, f.OpenTerm); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
		_, _, err := NewProgramRepository(pool).Update(ctx, f.ProgramID, models.ProgramUpdateInput{},
			models.PrerequisiteChanges{Added: []models.PrerequisiteTemplate{{Name: "Cover letter"}}})
		done <- err
	}()
	// Let the update reach the term lock the closing transaction holds.
	time.Sleep(500 * time.Millisecond)

	if _, err := closing.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		t.Fatal(err)
	}
	if _, err := closing.Exec(ctx, `SELECT 1 FROM applications WHERE program_term_id = $1 FOR UPDATE`, f.OpenTerm); err != nil {
		t.Fatalf("closing transaction could not lock the term's applications: %v", err)
	}
	if err := closing.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("prerequisite change: %v", err)
	}
}

// A program update holds the program before a prerequisite change key-share-locks its terms.
// Reopening a term must take the program first too, or the two can deadlock.
func TestProgramTermRepositoryIntegration_ReopenLocksProgramBeforeTerm(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)

	ctx := context.Background()
	editing, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = editing.Rollback(ctx) }()
	if _, err := editing.Exec(ctx, `SELECT 1 FROM programs WHERE id = $1 FOR UPDATE`, f.ProgramID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
		open := models.ProgramTermStatusOpen
		_, err := NewProgramTermRepository(pool).Update(ctx, f.ClosedTerm, models.ProgramTermUpdateInput{Status: &open})
		done <- err
	}()
	// Let the reopen reach the program lock the editing transaction holds.
	time.Sleep(500 * time.Millisecond)

	if _, err := editing.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		t.Fatal(err)
	}
	if _, err := editing.Exec(ctx, `SELECT 1 FROM program_terms WHERE program_id = $1 FOR KEY SHARE`, f.ProgramID); err != nil {
		t.Fatalf("editing transaction could not lock the program's terms: %v", err)
	}
	if err := editing.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("reopen term: %v", err)
	}
}

// Legacy clones are recognized by the program's lfid, so it can be set while empty but not changed,
// neither alongside a prerequisite change nor on its own.
func TestProgramRepositoryIntegration_UpdateKeepsLFIDOnceSet(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}
	repo := NewProgramRepository(pool)
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})

	creator, other := "legacy-creator", "someone-else"
	if _, _, err := repo.Update(ctx, f.ProgramID, models.ProgramUpdateInput{LFID: &creator}, models.PrerequisiteChanges{}); err != nil {
		t.Fatalf("set an empty lfid: %v", err)
	}
	if _, _, err := repo.Update(ctx, f.ProgramID, models.ProgramUpdateInput{LFID: &creator}, models.PrerequisiteChanges{}); err != nil {
		t.Fatalf("send the same lfid: %v", err)
	}
	if _, _, err := repo.Update(ctx, f.ProgramID, models.ProgramUpdateInput{LFID: &other}, models.PrerequisiteChanges{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("change lfid: err = %v; want ErrInvalidInput", err)
	}

	app := s.apply(f.OpenTerm, "mentee", "pending")
	reviewer := f.UserID
	s.createdBy(s.task(app, "Legacy", "prerequisite", "incomplete"), creator, &reviewer)
	if _, _, err := repo.Update(ctx, f.ProgramID, models.ProgramUpdateInput{LFID: &other},
		models.PrerequisiteChanges{Removed: []string{"legacy"}}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("change lfid with a removal: err = %v; want ErrInvalidInput", err)
	}
	if got := syncedTasks(t, pool, app); len(got) != 1 {
		t.Fatalf("tasks = %v; want the legacy clone kept when the update is refused", keys(got))
	}
	applyChanges(t, pool, f.ProgramID, models.PrerequisiteChanges{Removed: []string{"legacy"}})
	if got := syncedTasks(t, pool, app); len(got) != 0 {
		t.Errorf("tasks = %v; want the legacy clone removed, still recognized by the unchanged lfid", keys(got))
	}
}

// A template save holds the program before a prerequisite change key-share-locks its terms.
// Accepting a mentor, whose membership insert key-share-locks the program, must take the program
// before the term too, or the two can deadlock.
func TestApplicationRepositoryIntegration_MentorAcceptanceLocksProgramBeforeTerm(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}
	mentor := s.apply(f.OpenTerm, "mentor", "pending")

	ctx := context.Background()
	saving, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = saving.Rollback(ctx) }()
	if _, err := saving.Exec(ctx, `SELECT 1 FROM programs WHERE id = $1 FOR UPDATE`, f.ProgramID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
		accepted := models.ApplicationStatusAccepted
		_, err := NewApplicationRepository(pool).Update(ctx, mentor, models.ApplicationUpdateInput{Status: &accepted})
		done <- err
	}()
	// Let the acceptance reach the program lock the saving transaction holds.
	time.Sleep(500 * time.Millisecond)

	if _, err := saving.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		t.Fatal(err)
	}
	if _, err := saving.Exec(ctx, `SELECT 1 FROM program_terms WHERE program_id = $1 FOR KEY SHARE`, f.ProgramID); err != nil {
		t.Fatalf("saving transaction could not lock the program's terms: %v", err)
	}
	if err := saving.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("accept mentor: %v", err)
	}
}
