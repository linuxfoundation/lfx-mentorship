// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

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
		INSERT INTO tasks (id, application_id, program_term_id, assignee_id, name, description, category, status, due_date)
		SELECT $1, a.id, a.program_term_id, a.user_id, $2, 'Old description', $3, $4, '2020-01-01' FROM applications a WHERE a.id = $5`,
		id, name, category, status, appID); err != nil {
		s.t.Fatal(err)
	}
	return id
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
	_, flagged, err := NewProgramRepository(pool).UpdateWithPrerequisites(ctx, programID, models.ProgramUpdateInput{}, changes)
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
	s.task(pending, "Intro call", "non_prerequisite", "incomplete")
	// A pending application on a closed term takes the changes too.
	closedTerm := s.apply(f.ClosedTerm, "mentee", "pending")
	s.task(closedTerm, "Resume", "prerequisite", "complete")
	accepted := s.apply(f.OpenTerm, "mentee", "accepted")
	s.task(accepted, "Essay", "prerequisite", "incomplete")

	description, required, due := "Upload a PDF", "required", "2030-01-31"
	applyChanges(t, pool, f.ProgramID, models.PrerequisiteChanges{
		Removed: []string{"essay", "video", "notes"},
		Edited:  []models.PrerequisiteTemplate{{Name: "resume", Description: &description, SubmitFile: &required, DueDate: &due}},
		Added:   []models.PrerequisiteTemplate{{Name: "Cover letter", Custom: true}},
	})

	got := syncedTasks(t, pool, pending)
	want := map[string]string{
		"resume":       "incomplete",
		"Video":        "submitted",   // submitted work is kept
		"Notes":        "in_progress", // so is an uploaded file
		"Portfolio":    "incomplete",  // a prerequisite the edit did not touch
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
	if !got["Cover letter"].Custom {
		t.Errorf("added task lost its custom flag: %+v", got["Cover letter"])
	}
	for _, name := range []string{"Video", "Portfolio", "Intro call"} {
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
	if acc := syncedTasks(t, pool, accepted); len(acc) != 1 || deref(acc["Essay"].Description) != "Old description" {
		t.Errorf("accepted application's tasks were changed: %v", acc)
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

	flagged := applyChanges(t, pool, f.ProgramID, models.PrerequisiteChanges{Removed: []string{"essay"}})

	if len(flagged) != 1 || flagged[0] != app {
		t.Fatalf("flagged = %v; want only %s, whose last outstanding prerequisite was removed", flagged, app)
	}
	var submitted bool
	if err := pool.QueryRow(context.Background(), `SELECT tasks_submitted FROM applications WHERE id = $1`, app).Scan(&submitted); err != nil {
		t.Fatal(err)
	}
	if !submitted {
		t.Error("tasks_submitted = false; want true once the only outstanding prerequisite is removed")
	}
}

func TestProgramRepositoryIntegration_UpdateWithoutPrerequisiteChangesLeavesTasks(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &syncSeeder{t: t, pool: pool}

	app := s.apply(f.OpenTerm, "mentee", "pending")
	s.task(app, "Essay", "prerequisite", "incomplete")

	name := "Renamed Program"
	ctx := domain.ContextWithIndexHeaders(context.Background(), map[string]string{"authorization": "Bearer fixture"})
	if _, err := NewProgramRepository(pool).Update(ctx, f.ProgramID, models.ProgramUpdateInput{Name: &name, TaskTemplates: json.RawMessage(`[]`)}); err != nil {
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
