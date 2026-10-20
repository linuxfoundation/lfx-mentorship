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

// reminderSeeder inserts terms and applications for the prerequisite reminder query.
type reminderSeeder struct {
	t    *testing.T
	pool *pgxpool.Pool
	seq  int
}

func (s *reminderSeeder) exec(q string, args ...any) {
	s.t.Helper()
	if _, err := s.pool.Exec(context.Background(), q, args...); err != nil {
		s.t.Fatalf("%s: %v", q, err)
	}
}

func (s *reminderSeeder) term(programID, status string, start, end time.Time) string {
	s.t.Helper()
	s.seq++
	id := fmt.Sprintf("00000000-0000-0000-0000-%012d", 700+s.seq)
	s.exec(`INSERT INTO program_terms (id, program_id, name, status, application_start_date, application_end_date) VALUES ($1, $2, 'Term', $3, $4, $5)`,
		id, programID, status, start, end)
	return id
}

// apply inserts an application with its own user, since a user holds one live application per
// term and role, and a task per {category, status} pair.
func (s *reminderSeeder) apply(termID, role, status string, createdOn time.Time, tasks ...[2]string) string {
	s.t.Helper()
	s.seq++
	userID := fmt.Sprintf("00000000-0000-0000-0001-%012d", s.seq)
	appID := fmt.Sprintf("00000000-0000-0000-0002-%012d", s.seq)
	s.exec(`INSERT INTO users (id, lfid, name) VALUES ($1, $2, 'Applicant')`, userID, fmt.Sprintf("applicant-%d", s.seq))
	s.exec(`INSERT INTO applications (id, program_term_id, user_id, role, status, created_on) VALUES ($1, $2, $3, $4, $5, $6)`,
		appID, termID, userID, role, status, createdOn)
	for i, task := range tasks {
		s.exec(`INSERT INTO tasks (id, application_id, program_term_id, assignee_id, name, category, status) VALUES ($1, $2, $3, $4, 'Task', $5, $6)`,
			fmt.Sprintf("00000000-0000-0000-%04d-%012d", 3+i, s.seq), appID, termID, userID, task[0], task[1])
	}
	return appID
}

func listRemindersDue(t *testing.T, pool *pgxpool.Pool, now time.Time) []*models.PrerequisiteReminderCandidate {
	t.Helper()
	due, err := NewApplicationRepository(pool).ListPrerequisiteRemindersDue(context.Background(), now)
	if err != nil {
		t.Fatalf("ListPrerequisiteRemindersDue: %v", err)
	}
	return due
}

var (
	incompletePrerequisite = [2]string{"prerequisite", "incomplete"}
	inProgressPrerequisite = [2]string{"prerequisite", "in_progress"}
	submittedPrerequisite  = [2]string{"prerequisite", "submitted"}
	completePrerequisite   = [2]string{"prerequisite", "complete"}
	incompleteOtherTask    = [2]string{"non_prerequisite", "incomplete"}
)

func TestApplicationRepositoryIntegration_ListPrerequisiteRemindersDueFiltersApplications(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &reminderSeeder{t: t, pool: pool}

	const draftProgram = "00000000-0000-0000-0000-000000000799"
	s.exec(`INSERT INTO programs (id, lf_project_uid, name, slug, status) VALUES ($1, '00000000-0000-0000-0000-000000000098', 'Draft Program', 'draft-program', 'pending')`, draftProgram)

	now := time.Now()
	day := 24 * time.Hour
	open := s.term(f.ProgramID, "open", now.Add(-30*day), now.Add(30*day))
	ended := s.term(f.ProgramID, "open", now.Add(-30*day), now.Add(-day))
	upcoming := s.term(f.ProgramID, "open", now.Add(day), now.Add(30*day))
	draft := s.term(draftProgram, "open", now.Add(-30*day), now.Add(30*day))
	// Every application is on its third day, a reminder day, so only the filters decide.
	applied := now.Add(-3 * day)

	want := []string{
		s.apply(open, "mentee", "pending", applied, incompletePrerequisite),
		s.apply(open, "mentee", "pending", applied, submittedPrerequisite, inProgressPrerequisite),
	}
	s.apply(open, "mentee", "pending", applied, submittedPrerequisite, completePrerequisite)
	s.apply(open, "mentee", "pending", applied, incompleteOtherTask)
	s.apply(open, "mentee", "pending", applied)
	s.apply(open, "mentee", "accepted", applied, incompletePrerequisite)
	s.apply(open, "mentee", "declined", applied, incompletePrerequisite)
	s.apply(open, "mentor", "pending", applied, incompletePrerequisite)
	s.apply(f.ClosedTerm, "mentee", "pending", applied, incompletePrerequisite)
	s.apply(ended, "mentee", "pending", applied, incompletePrerequisite)
	s.apply(upcoming, "mentee", "pending", applied, incompletePrerequisite)
	s.apply(draft, "mentee", "pending", applied, incompletePrerequisite)

	var got []string
	for _, c := range listRemindersDue(t, pool, now) {
		got = append(got, c.ApplicationID)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("reminders due = %v; want %v", got, want)
	}
}

// The query applies the reminder schedule in SQL; it must agree with models.IsPrerequisiteReminderDue,
// the unit-tested rule, on every application and close day around each boundary.
func TestApplicationRepositoryIntegration_ListPrerequisiteRemindersDueMatchesSchedule(t *testing.T) {
	pool := integrationPool(t)
	f := seedIntegrationFixture(t, pool)
	s := &reminderSeeder{t: t, pool: pool}

	now := time.Now().UTC()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour

	var want []string
	for closesIn := 0; closesIn <= 5; closesIn++ {
		// Late in the day, so a close date is compared by its UTC day and not its time.
		end := today.Add(time.Duration(closesIn)*day + 23*time.Hour)
		term := s.term(f.ProgramID, "open", today.Add(-60*day), end)
		for appliedAgo := 0; appliedAgo <= 25; appliedAgo++ {
			// Early in the day, so a whole UTC day passes between it and now only at midnight.
			createdOn := today.Add(-time.Duration(appliedAgo)*day + time.Minute)
			id := s.apply(term, "mentee", "pending", createdOn, incompletePrerequisite)
			if models.IsPrerequisiteReminderDue(createdOn, end, now) {
				want = append(want, id)
			}
		}
	}

	var got []string
	for _, c := range listRemindersDue(t, pool, now) {
		got = append(got, c.ApplicationID)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("reminders due = %v; want %v", got, want)
	}
}
