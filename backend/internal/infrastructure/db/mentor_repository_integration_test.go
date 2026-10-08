// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// mentoredSeed inserts rows for the mentor programs list tests. IDs are derived
// from short labels so a test reads as data rather than UUID literals.
type mentoredSeed struct {
	t    *testing.T
	pool *pgxpool.Pool
	ids  map[string]string
}

func newMentoredSeed(t *testing.T, pool *pgxpool.Pool) *mentoredSeed {
	return &mentoredSeed{t: t, pool: pool, ids: map[string]string{}}
}

// id returns a stable UUID for label, allocating one on first use.
func (s *mentoredSeed) id(label string) string {
	if id, ok := s.ids[label]; ok {
		return id
	}
	id := fmt.Sprintf("00000000-0000-0000-0001-%012d", len(s.ids)+1)
	s.ids[label] = id
	return id
}

func (s *mentoredSeed) exec(q string, args ...any) {
	s.t.Helper()
	if _, err := s.pool.Exec(context.Background(), q, args...); err != nil {
		s.t.Fatalf("seed %q: %v", q, err)
	}
}

func (s *mentoredSeed) user(label string) string {
	s.t.Helper()
	s.exec(`INSERT INTO users (id, lfid, name) VALUES ($1, $2, $2)`, s.id(label), label)
	return s.id(label)
}

func (s *mentoredSeed) program(label, name, status string) string {
	s.t.Helper()
	s.exec(`INSERT INTO programs (id, lf_project_uid, lf_project_name, name, slug, status)
		VALUES ($1, '00000000-0000-0000-0000-000000000099', $2, $3, $4, $5)`,
		s.id(label), name+" Project", name, label, status)
	return s.id(label)
}

func (s *mentoredSeed) member(program, user, memberType, status string) {
	s.t.Helper()
	s.exec(`INSERT INTO program_members (id, program_id, user_id, member_type, status) VALUES ($1, $2, $3, $4, $5)`,
		s.id(program+"/"+user+"/"+memberType), s.id(program), s.id(user), memberType, status)
}

func (s *mentoredSeed) term(program, label, status string, start *time.Time) string {
	s.t.Helper()
	s.exec(`INSERT INTO program_terms (id, program_id, name, status, start_date_time) VALUES ($1, $2, $3, $4, $5)`,
		s.id(label), s.id(program), label, status, start)
	return s.id(label)
}

func (s *mentoredSeed) application(label, term, user, role, status string, createdOn time.Time) string {
	s.t.Helper()
	s.exec(`INSERT INTO applications (id, program_term_id, user_id, role, status, created_on) VALUES ($1, $2, $3, $4, $5, $6)`,
		s.id(label), s.id(term), s.id(user), role, status, createdOn)
	return s.id(label)
}

func (s *mentoredSeed) task(label, application, term, user, status string) {
	s.t.Helper()
	s.exec(`INSERT INTO tasks (id, application_id, program_term_id, assignee_id, status) VALUES ($1, $2, $3, $4, $5)`,
		s.id(label), s.id(application), s.id(term), s.id(user), status)
}

func daysFromNow(days int) *time.Time {
	t := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	return &t
}

func TestMentorRepositoryIntegration_ListMentoredByUserScopesToActivePublished(t *testing.T) {
	pool := integrationPool(t)
	seed := newMentoredSeed(t, pool)
	seed.user("mentor")
	seed.user("other")

	seed.program("mine", "Mine", "published")
	seed.member("mine", "mentor", "mentor", "active")
	seed.program("unpublished", "Unpublished", "pending")
	seed.member("unpublished", "mentor", "mentor", "active")
	seed.program("invited", "Invited", "published")
	seed.member("invited", "mentor", "mentor", "invited")
	seed.program("admin-only", "Admin Only", "published")
	seed.member("admin-only", "mentor", "program_admin", "active")
	seed.program("theirs", "Theirs", "published")
	seed.member("theirs", "other", "mentor", "active")

	repo := NewMentorRepository(pool)
	got, meta, err := repo.ListMentoredByUser(context.Background(), seed.id("mentor"), models.MentoredProgramFilter{})
	if err != nil {
		t.Fatalf("ListMentoredByUser: %v", err)
	}
	if len(got) != 1 || got[0].ID != seed.id("mine") || meta.Total != 1 {
		t.Fatalf("got %d programs (total %d); want only the active, published mentor program", len(got), meta.Total)
	}
	if got[0].ProjectName == nil || *got[0].ProjectName != "Mine Project" || got[0].Slug != "mine" {
		t.Errorf("row = %+v; want slug and project name of the program", got[0])
	}

	none, meta, err := repo.ListMentoredByUser(context.Background(), seed.user("nobody"), models.MentoredProgramFilter{})
	if err != nil {
		t.Fatalf("ListMentoredByUser for a non-mentor: %v", err)
	}
	if none == nil || len(none) != 0 || meta.Total != 0 || meta.Limit != 20 {
		t.Errorf("got %v meta %+v; want an empty, non-nil page with the default limit", none, meta)
	}
}

// A program is completed once all its terms are closed and open otherwise, the
// same grouping as admin_status, so the admin and mentor lists agree on it.
func TestMentorRepositoryIntegration_ListMentoredByUserStatusAndOrder(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	seed := newMentoredSeed(t, pool)
	seed.user("both")
	both := func(label, name string) {
		seed.program(label, name, "published")
		seed.member(label, "both", "program_admin", "active")
		seed.member(label, "both", "mentor", "active")
	}

	both("running", "delta")
	seed.term("running", "running-open", "open", daysFromNow(-10))
	both("no-terms", "Bravo")
	both("mixed", "charlie")
	seed.term("mixed", "mixed-closed", "closed", daysFromNow(-200))
	seed.term("mixed", "mixed-open", "open", daysFromNow(30))
	both("deleted-only", "echo")
	seed.term("deleted-only", "deleted-only-term", "deleted", daysFromNow(-5))
	both("done", "foxtrot")
	seed.term("done", "done-closed", "closed", daysFromNow(-100))
	seed.term("done", "done-deleted", "deleted", daysFromNow(-1))
	both("done-early", "Alpha")
	seed.term("done-early", "done-early-closed", "closed", daysFromNow(-300))

	mentored, meta, err := NewMentorRepository(pool).ListMentoredByUser(ctx, seed.id("both"), models.MentoredProgramFilter{})
	if err != nil {
		t.Fatalf("ListMentoredByUser: %v", err)
	}
	type row struct {
		program string
		status  models.MentoredProgramStatus
	}
	want := []row{
		{"no-terms", models.MentoredProgramStatusOpen},
		{"mixed", models.MentoredProgramStatusOpen},
		{"running", models.MentoredProgramStatusOpen},
		{"deleted-only", models.MentoredProgramStatusOpen},
		{"done-early", models.MentoredProgramStatusCompleted},
		{"done", models.MentoredProgramStatusCompleted},
	}
	if meta.Total != len(want) || len(mentored) != len(want) {
		t.Fatalf("got %d rows (total %d); want %d", len(mentored), meta.Total, len(want))
	}
	for i, w := range want {
		if g := mentored[i]; g.ID != seed.id(w.program) || g.Status != w.status {
			t.Errorf("row %d = program %s status %s; want program %s (%s) status %s",
				i, g.ID, g.Status, seed.id(w.program), w.program, w.status)
		}
	}

	administered, _, err := NewProgramRepository(pool).ListAdministeredByUser(ctx, seed.id("both"), models.AdministeredProgramFilter{})
	if err != nil {
		t.Fatalf("ListAdministeredByUser: %v", err)
	}
	mentorStatus := map[string]models.MentoredProgramStatus{}
	for _, p := range mentored {
		mentorStatus[p.ID] = p.Status
	}
	for _, p := range administered {
		if string(p.AdminStatus) != string(mentorStatus[p.ID]) {
			t.Errorf("program %s: admin_status %s, mentor status %s; want them equal", p.ID, p.AdminStatus, mentorStatus[p.ID])
		}
	}
}

func TestMentorRepositoryIntegration_ListMentoredByUserCountsWholeProgram(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	seed := newMentoredSeed(t, pool)
	for _, u := range []string{"mentor", "accepted", "graduated", "pending", "reapplied", "withdrawn", "past", "mentor-applicant", "elsewhere"} {
		seed.user(u)
	}
	program := seed.program("counted", "Counted", "published")
	seed.member("counted", "mentor", "mentor", "active")
	seed.term("counted", "current", "open", daysFromNow(-7))
	seed.term("counted", "previous", "closed", daysFromNow(-120))
	base := time.Now().Add(-48 * time.Hour)

	// Current term. An accepted mentee: both submitted tasks count, the complete one does not.
	seed.application("app-accepted", "current", "accepted", "mentee", "accepted", base)
	seed.task("task-accepted-1", "app-accepted", "current", "accepted", "submitted")
	seed.task("task-accepted-2", "app-accepted", "current", "accepted", "submitted")
	seed.task("task-accepted-3", "app-accepted", "current", "accepted", "complete")
	// A graduated mentee: a mentee, but a leftover submission is not to review.
	seed.application("app-graduated", "current", "graduated", "mentee", "graduated", base)
	seed.task("task-graduated", "app-graduated", "current", "graduated", "submitted")
	// A pending applicant: an applicant only.
	seed.application("app-pending", "current", "pending", "mentee", "pending", base)
	seed.task("task-pending", "app-pending", "current", "pending", "submitted")
	// Withdrawn twice, then reapplied: one applicant.
	seed.application("app-reapplied-1", "current", "reapplied", "mentee", "withdrawn", base)
	seed.application("app-reapplied-2", "current", "reapplied", "mentee", "withdrawn", base.Add(time.Hour))
	seed.application("app-reapplied-3", "current", "reapplied", "mentee", "pending", base.Add(2*time.Hour))
	// Withdrawn twice with no reapplication: still one applicant.
	seed.application("app-withdrawn-1", "current", "withdrawn", "mentee", "withdrawn", base)
	seed.application("app-withdrawn-2", "current", "withdrawn", "mentee", "withdrawn", base.Add(time.Hour))
	// A mentor-role application and its prerequisite task are not counted.
	seed.application("app-mentor", "current", "mentor-applicant", "mentor", "accepted", base)
	seed.task("task-mentor", "app-mentor", "current", "mentor-applicant", "submitted")

	// Previous term: counted too. The accepted mentee above also graduated
	// here, so they are an applicant and a mentee once per term.
	seed.application("app-past", "previous", "past", "mentee", "accepted", base)
	seed.task("task-past", "app-past", "previous", "past", "submitted")
	seed.application("app-accepted-before", "previous", "accepted", "mentee", "graduated", base)

	// Another program's applications are not counted.
	seed.program("other", "Other", "published")
	seed.term("other", "other-term", "open", daysFromNow(-7))
	seed.application("app-elsewhere", "other-term", "elsewhere", "mentee", "accepted", base)
	seed.task("task-elsewhere", "app-elsewhere", "other-term", "elsewhere", "submitted")

	got, _, err := NewMentorRepository(pool).ListMentoredByUser(ctx, seed.id("mentor"), models.MentoredProgramFilter{})
	if err != nil {
		t.Fatalf("ListMentoredByUser: %v", err)
	}
	if len(got) != 1 || got[0].ID != program {
		t.Fatalf("got %+v; want only the counted program", got)
	}
	want := models.MentoredProgramStats{Mentees: 4, Applicants: 7, TasksToReview: 3}
	if got[0].Stats != want {
		t.Errorf("stats = %+v; want %+v", got[0].Stats, want)
	}

	// The card's applicant count agrees with the program's management summary.
	summary, err := NewProgramRepository(pool).GetManagementSummary(ctx, program)
	if err != nil {
		t.Fatalf("GetManagementSummary: %v", err)
	}
	if summary.Applicants != want.Applicants {
		t.Errorf("management summary applicants = %d; want %d", summary.Applicants, want.Applicants)
	}
}

func TestMentorRepositoryIntegration_ListMentoredByUserPages(t *testing.T) {
	pool := integrationPool(t)
	seed := newMentoredSeed(t, pool)
	seed.user("mentor")
	for _, name := range []string{"Alpha", "Bravo", "Charlie"} {
		seed.program(name, name, "published")
		seed.member(name, "mentor", "mentor", "active")
	}

	got, meta, err := NewMentorRepository(pool).ListMentoredByUser(context.Background(), seed.id("mentor"), models.MentoredProgramFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("ListMentoredByUser: %v", err)
	}
	if len(got) != 1 || got[0].ID != seed.id("Bravo") {
		t.Errorf("page = %+v; want only Bravo", got)
	}
	if meta.Total != 3 || meta.Limit != 1 || meta.Offset != 1 {
		t.Errorf("meta = %+v; want total 3, limit 1, offset 1", meta)
	}
}
