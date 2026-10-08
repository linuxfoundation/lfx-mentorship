// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"fmt"
	"slices"
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

func TestMentorRepositoryIntegration_ListMentoredByUserChoosesTermAndOrders(t *testing.T) {
	pool := integrationPool(t)
	seed := newMentoredSeed(t, pool)
	seed.user("mentor")
	mentor := func(label, name string) {
		seed.program(label, name, "published")
		seed.member(label, "mentor", "mentor", "active")
	}

	// The open term that started most recently wins over older, future, and closed terms.
	mentor("started", "delta")
	seed.term("started", "started-old", "open", daysFromNow(-30))
	seed.term("started", "started-new", "open", daysFromNow(-2))
	seed.term("started", "started-future", "open", daysFromNow(10))
	seed.term("started", "started-closed", "closed", daysFromNow(-1))
	seed.term("started", "started-deleted", "deleted", daysFromNow(-1))

	// Two open terms that started together: the lower id wins.
	mentor("tied", "Alpha")
	tiedStart := daysFromNow(-5)
	seed.term("tied", "tied-first", "open", tiedStart)
	seed.term("tied", "tied-second", "open", tiedStart)

	// No started open term: the first to start wins, undated last; closed terms lose.
	mentor("future", "charlie")
	seed.term("future", "future-late", "open", daysFromNow(30))
	seed.term("future", "future-undated", "open", nil)
	seed.term("future", "future-soon", "open", daysFromNow(5))
	seed.term("future", "future-closed", "closed", daysFromNow(-60))

	// Only an undated open term (and a deleted one): it is upcoming.
	mentor("undated", "Bravo")
	seed.term("undated", "undated-open", "open", nil)
	seed.term("undated", "undated-deleted", "deleted", daysFromNow(-3))

	// Only closed terms: the latest to start wins; a deleted term is never chosen.
	mentor("closed", "echo")
	seed.term("closed", "closed-old", "closed", daysFromNow(-200))
	seed.term("closed", "closed-new", "closed", daysFromNow(-100))
	seed.term("closed", "closed-deleted", "deleted", daysFromNow(-1))

	// No terms at all: upcoming, with no term.
	mentor("empty", "Foxtrot")

	got, meta, err := NewMentorRepository(pool).ListMentoredByUser(context.Background(), seed.id("mentor"), models.MentoredProgramFilter{})
	if err != nil {
		t.Fatalf("ListMentoredByUser: %v", err)
	}
	type row struct {
		program, term string
		status        models.MentoredProgramTermStatus
	}
	want := []row{
		{"tied", "tied-first", models.MentoredProgramTermStatusActiveTerm},
		{"started", "started-new", models.MentoredProgramTermStatusActiveTerm},
		{"undated", "undated-open", models.MentoredProgramTermStatusUpcoming},
		{"future", "future-soon", models.MentoredProgramTermStatusUpcoming},
		{"empty", "", models.MentoredProgramTermStatusUpcoming},
		{"closed", "closed-new", models.MentoredProgramTermStatusCompleted},
	}
	if meta.Total != len(want) || len(got) != len(want) {
		t.Fatalf("got %d rows (total %d); want %d", len(got), meta.Total, len(want))
	}
	for i, w := range want {
		g := got[i]
		termID := ""
		if g.Term != nil {
			termID = g.Term.ID
		}
		wantTermID := ""
		if w.term != "" {
			wantTermID = seed.id(w.term)
		}
		if g.ID != seed.id(w.program) || termID != wantTermID || g.TermStatus != w.status {
			t.Errorf("row %d = program %s term %q status %s; want program %s term %s status %s",
				i, g.ID, termID, g.TermStatus, seed.id(w.program), w.term, w.status)
		}
	}
	if empty := got[4]; empty.Stats != (models.MentoredProgramStats{}) {
		t.Errorf("program with no terms has stats %+v; want zero", empty.Stats)
	}
}

func TestMentorRepositoryIntegration_ListMentoredByUserCountsChosenTermOnly(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	seed := newMentoredSeed(t, pool)
	for _, u := range []string{"mentor", "accepted", "graduated", "pending", "reapplied", "withdrawn", "past", "mentor-applicant"} {
		seed.user(u)
	}
	program := seed.program("counted", "Counted", "published")
	seed.member("counted", "mentor", "mentor", "active")
	current := seed.term("counted", "current", "open", daysFromNow(-7))
	seed.term("counted", "previous", "closed", daysFromNow(-120))
	base := time.Now().Add(-48 * time.Hour)

	// An accepted mentee: both submitted tasks count, the complete one does not.
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
	// A mentee on the previous term is not counted.
	seed.application("app-past", "previous", "past", "mentee", "accepted", base)
	seed.task("task-past", "app-past", "previous", "past", "submitted")
	// A mentor-role application and its prerequisite task are not counted.
	seed.application("app-mentor", "current", "mentor-applicant", "mentor", "accepted", base)
	seed.task("task-mentor", "app-mentor", "current", "mentor-applicant", "submitted")

	got, _, err := NewMentorRepository(pool).ListMentoredByUser(ctx, seed.id("mentor"), models.MentoredProgramFilter{})
	if err != nil {
		t.Fatalf("ListMentoredByUser: %v", err)
	}
	if len(got) != 1 || got[0].Term == nil || got[0].Term.ID != current {
		t.Fatalf("got %+v; want one program on the current term", got)
	}
	want := models.MentoredProgramStats{Mentees: 2, Applicants: 5, TasksToReview: 2}
	if got[0].Stats != want {
		t.Errorf("stats = %+v; want %+v", got[0].Stats, want)
	}

	// The card's counts agree with the program's applications list for the term.
	rows, meta, err := NewApplicationRepository(pool).ListByProgram(ctx, program, models.ProgramApplicationFilter{TermID: current, Limit: 50})
	if err != nil {
		t.Fatalf("ListByProgram: %v", err)
	}
	mentees := 0
	for _, r := range rows {
		if slices.Contains([]models.ApplicationStatus{models.ApplicationStatusAccepted, models.ApplicationStatusGraduated}, r.Status) {
			mentees++
		}
	}
	if meta.Total != want.Applicants || mentees != want.Mentees {
		t.Errorf("applications list = %d applicants, %d mentees; want %d and %d", meta.Total, mentees, want.Applicants, want.Mentees)
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

// The admin and mentor programs lists share chosenProgramTermJoin, so they show
// the same term for the same program.
func TestProgramListsIntegration_AdminAndMentorShareTermChoice(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	seed := newMentoredSeed(t, pool)
	seed.user("both")
	both := func(label, name string) {
		seed.program(label, name, "published")
		seed.member(label, "both", "program_admin", "active")
		seed.member(label, "both", "mentor", "active")
	}

	// A started term beats a later-starting future one.
	both("running", "Alpha")
	seed.term("running", "running-started", "open", daysFromNow(-10))
	seed.term("running", "running-future", "open", daysFromNow(60))
	// With nothing started, the first future term to start wins.
	both("waiting", "Bravo")
	seed.term("waiting", "waiting-late", "open", daysFromNow(90))
	seed.term("waiting", "waiting-soon", "open", daysFromNow(15))
	// With only closed terms, the latest to start wins.
	both("finished", "Charlie")
	seed.term("finished", "finished-old", "closed", daysFromNow(-300))
	seed.term("finished", "finished-new", "closed", daysFromNow(-150))

	want := map[string]string{
		seed.id("running"):  seed.id("running-started"),
		seed.id("waiting"):  seed.id("waiting-soon"),
		seed.id("finished"): seed.id("finished-new"),
	}

	administered, _, err := NewProgramRepository(pool).ListAdministeredByUser(ctx, seed.id("both"), models.AdministeredProgramFilter{})
	if err != nil {
		t.Fatalf("ListAdministeredByUser: %v", err)
	}
	mentored, _, err := NewMentorRepository(pool).ListMentoredByUser(ctx, seed.id("both"), models.MentoredProgramFilter{})
	if err != nil {
		t.Fatalf("ListMentoredByUser: %v", err)
	}
	if len(administered) != len(want) || len(mentored) != len(want) {
		t.Fatalf("got %d administered and %d mentored programs; want %d each", len(administered), len(mentored), len(want))
	}
	for _, p := range administered {
		if p.Term == nil || p.Term.ID != want[p.ID] {
			t.Errorf("admin list program %s term = %+v; want %s", p.ID, p.Term, want[p.ID])
		}
		wantStatus := models.AdministeredProgramStatusOpen
		if p.ID == seed.id("finished") {
			wantStatus = models.AdministeredProgramStatusCompleted
		}
		if p.AdminStatus != wantStatus {
			t.Errorf("admin list program %s admin_status = %s; want %s", p.ID, p.AdminStatus, wantStatus)
		}
	}
	for _, p := range mentored {
		if p.Term == nil || p.Term.ID != want[p.ID] {
			t.Errorf("mentor list program %s term = %+v; want %s", p.ID, p.Term, want[p.ID])
		}
	}
}
