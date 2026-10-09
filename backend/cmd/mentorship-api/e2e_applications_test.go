// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"encoding/csv"
	"net/http"
	"strings"
	"testing"
)

type application struct {
	ID             string  `json:"id"`
	ProgramTermID  string  `json:"program_term_id"`
	UserID         string  `json:"user_id"`
	Role           string  `json:"role"`
	Status         string  `json:"status"`
	AttendanceType *string `json:"attendance_type"`
	TasksSubmitted bool    `json:"tasks_submitted"`
}

type task struct {
	ID            string  `json:"id"`
	ApplicationID *string `json:"application_id"`
	AssigneeID    string  `json:"assignee_id"`
	Name          string  `json:"name"`
	Category      string  `json:"category"`
	Status        string  `json:"status"`
	SubmitFile    *string `json:"submit_file"`
	File          *string `json:"file"`
	DueDate       *string `json:"due_date"`
	Custom        bool    `json:"custom"`
}

func applicationsPath(p *program) string {
	return "/programs/" + p.ID + "/terms/" + p.TermID + "/applications"
}

// apply submits a mentee application for p's open term.
func apply(t *testing.T, mentee *actor, p *program) application {
	t.Helper()
	var app application
	mentee.mustJSON(t, http.MethodPost, applicationsPath(p), map[string]any{"role": "mentee"}, http.StatusCreated, &app)
	return app
}

// tasksOf lists an application's tasks as the caller.
func tasksOf(t *testing.T, a *actor, applicationID string) []task {
	t.Helper()
	var page struct {
		Data []task `json:"data"`
	}
	a.mustJSON(t, http.MethodGet, "/applications/"+applicationID+"/tasks", nil, http.StatusOK, &page)
	return page.Data
}

func TestE2EMenteeJourney(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Journey Program")
	mentor := signIn(t, "mentor")
	inviteMentor(t, p, mentor)
	mentee := signIn(t, "mentee")
	registerMentee(t, mentee)

	var app application
	mentee.mustJSON(t, http.MethodPost, applicationsPath(p), map[string]any{"role": "mentee", "attendance_type": "full_time", "user_id": admin.ID}, http.StatusCreated, &app)
	if app.Status != "pending" || app.UserID != mentee.ID || app.AttendanceType != nil {
		t.Fatalf("application: %+v", app)
	}

	tasks := tasksOf(t, mentee, app.ID)
	if len(tasks) != 1 || tasks[0].Category != "prerequisite" || tasks[0].Status != "incomplete" || tasks[0].SubmitFile == nil {
		t.Fatalf("prerequisite tasks: %+v", tasks)
	}
	prereq := tasks[0]

	t.Run("the applicant sees their application", func(t *testing.T) {
		var mine struct {
			Data []application `json:"data"`
		}
		mentee.mustJSON(t, http.MethodGet, "/me/applications?role=mentee&status=pending", nil, http.StatusOK, &mine)
		if len(mine.Data) != 1 || mine.Data[0].ID != app.ID {
			t.Fatalf("my applications: %+v", mine)
		}
		mentee.call(t, http.MethodGet, "/me/applications?limit=x", nil).expect(http.StatusBadRequest)
		var got application
		mentee.mustJSON(t, http.MethodGet, "/applications/"+app.ID, nil, http.StatusOK, &got)
		mentee.call(t, http.MethodGet, "/applications/00000000-0000-4000-8000-000000000000", nil).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodGet, "/applications/"+app.ID, nil).expect(http.StatusUnauthorized)
	})

	t.Run("the applicant edits only applicant fields", func(t *testing.T) {
		var updated struct {
			StartDateTime string `json:"start_date_time"`
		}
		mentee.mustJSON(t, http.MethodPatch, "/applications/"+app.ID, map[string]any{"start_date_time": at(20)}, http.StatusOK, &updated)
		if updated.StartDateTime == "" {
			t.Fatalf("start date not saved")
		}
		for _, field := range []string{"status", "tasks_submitted", "admin_notified", "evaluation", "reviewer_note", "attendance_type", "program_term_status"} {
			var value any = "x"
			if field == "tasks_submitted" || field == "admin_notified" {
				value = true
			}
			mentee.call(t, http.MethodPatch, "/applications/"+app.ID, map[string]any{field: value}).expect(http.StatusBadRequest)
		}
	})

	t.Run("submitting the prerequisite", func(t *testing.T) {
		mentee.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "complete"}).expect(http.StatusConflict)
		mentee.mustJSON(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "in_progress"}, http.StatusOK, &prereq)
		if prereq.Status != "in_progress" {
			t.Fatalf("after in_progress: %+v", prereq)
		}
		mentee.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "in_progress"}).expect(http.StatusConflict)
		// The task requires a file, so it cannot be submitted without one.
		mentee.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "submitted"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "submitted"}).expect(http.StatusBadRequest)
		uploadTaskFile(t, mentee, prereq.ID, "intro.pdf", pdfBytes()).expect(http.StatusCreated)
		mentor.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "submitted"}).expect(http.StatusForbidden)
		mentee.mustJSON(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "submitted"}, http.StatusOK, &prereq)

		var got application
		mentee.mustJSON(t, http.MethodGet, "/applications/"+app.ID, nil, http.StatusOK, &got)
		if !got.TasksSubmitted {
			t.Fatalf("application not flagged tasks_submitted")
		}
		msg := awaitEmail(t, admin.Email, "New mentee application to review", 1)
		if !strings.Contains(msg.Text, "Spring") {
			t.Fatalf("tasks-submitted email lacks the term: %s", msg.Text)
		}
		// Only active program admins are told; mentors are not.
		if n := len(e2e.fakes.emailsTo(mentor.Email)); n != 1 {
			t.Fatalf("mentor got %d emails; want only the invite", n)
		}
	})

	t.Run("submission and review validation", func(t *testing.T) {
		mentee.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{}).expect(http.StatusBadRequest)
		mentee.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "in_progress", "file": "x"}).expect(http.StatusBadRequest)
		mentee.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "bogus"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/review", map[string]any{}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/review", map[string]any{"status": "in_progress"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/review", map[string]any{"application_status": "bogus"}).expect(http.StatusBadRequest)
		// Nobody reviews their own task.
		mentee.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/review", map[string]any{"status": "complete"}).expect(http.StatusForbidden)
		mentee.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/review", map[string]any{"application_status": "pending"}).expect(http.StatusForbidden)
		mentee.call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/submission", map[string]any{"status": "complete"}).expect(http.StatusForbidden)
		mentor.call(t, http.MethodPatch, "/tasks/00000000-0000-4000-8000-000000000000/review", map[string]any{"status": "complete"}).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodPatch, "/tasks/"+prereq.ID+"/review", map[string]any{"status": "complete"}).expect(http.StatusUnauthorized)
	})

	t.Run("a submitted file can be replaced but not removed", func(t *testing.T) {
		mentee.call(t, http.MethodDelete, "/tasks/"+prereq.ID+"/file", nil).expect(http.StatusConflict)
		uploadTaskFile(t, mentee, prereq.ID, "intro-v2.txt", []byte("plain text submission\n")).expect(http.StatusCreated)
	})

	mentor.mustJSON(t, http.MethodPatch, "/tasks/"+prereq.ID+"/review", map[string]any{"status": "complete", "application_status": "pending", "program_term_status": "open"}, http.StatusOK, &prereq)
	if prereq.Status != "complete" {
		t.Fatalf("after review: %+v", prereq)
	}
	uploadTaskFile(t, mentee, prereq.ID, "late.pdf", pdfBytes()).expect(http.StatusConflict)

	t.Run("reviewer notes and evaluation", func(t *testing.T) {
		var eval struct {
			Evaluation string `json:"evaluation"`
		}
		mentor.mustJSON(t, http.MethodPut, "/applications/"+app.ID+"/evaluation", map[string]any{"evaluation": "Strong candidate"}, http.StatusOK, &eval)
		if eval.Evaluation != "Strong candidate" {
			t.Fatalf("evaluation: %+v", eval)
		}
		mentor.call(t, http.MethodPut, "/applications/"+app.ID+"/evaluation", map[string]any{"evaluation": "x", "status": "accepted"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPut, "/applications/"+app.ID+"/evaluation", map[string]any{}).expect(http.StatusBadRequest)
		var note struct {
			Note *string `json:"note"`
		}
		admin.mustJSON(t, http.MethodGet, "/applications/"+app.ID+"/note", nil, http.StatusOK, &note)
		if note.Note != nil {
			t.Fatalf("note before write: %v", *note.Note)
		}
		admin.mustJSON(t, http.MethodPut, "/applications/"+app.ID+"/note", map[string]any{"reviewer_note": "Interview done"}, http.StatusOK, &note)
		admin.mustJSON(t, http.MethodGet, "/applications/"+app.ID+"/note", nil, http.StatusOK, &note)
		if note.Note == nil || *note.Note != "Interview done" {
			t.Fatalf("note: %+v", note)
		}
		admin.call(t, http.MethodPut, "/applications/"+app.ID+"/note", map[string]any{"reviewer_note": "x", "evaluation": "y"}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodGet, "/applications/00000000-0000-4000-8000-000000000000/note", nil).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodGet, "/applications/"+app.ID+"/note", nil).expect(http.StatusUnauthorized)
		anonymous().call(t, http.MethodPut, "/applications/"+app.ID+"/evaluation", map[string]any{"evaluation": "x"}).expect(http.StatusUnauthorized)
	})

	t.Run("program-level application views", func(t *testing.T) {
		var rows struct {
			Data []struct {
				ApplicationID  string `json:"application_id"`
				Status         string `json:"status"`
				TasksSubmitted int    `json:"tasks_submitted"`
				TasksTotal     int    `json:"tasks_total"`
				Note           string `json:"note"`
			} `json:"data"`
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/applications", nil, http.StatusOK, &rows)
		if len(rows.Data) != 1 || rows.Data[0].ApplicationID != app.ID || rows.Data[0].TasksTotal != 1 || rows.Data[0].Note != "Interview done" {
			t.Fatalf("program applications: %+v", rows)
		}
		for _, q := range []string{"type=current", "type=all&status=tasks_submitted", "status=pending&term=" + p.TermID, "search=mentee"} {
			admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/applications?"+q, nil, http.StatusOK, &rows)
			if len(rows.Data) != 1 {
				t.Fatalf("?%s: %+v", q, rows)
			}
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/applications?type=past", nil, http.StatusOK, &rows)
		if len(rows.Data) != 0 {
			t.Fatalf("past: %+v", rows)
		}
		admin.call(t, http.MethodGet, "/programs/"+p.ID+"/applications?type=future", nil).expect(http.StatusBadRequest)
		admin.call(t, http.MethodGet, "/programs/"+p.ID+"/applications?status=bogus", nil).expect(http.StatusBadRequest)
		admin.call(t, http.MethodGet, "/programs/"+p.ID+"/applications?limit=x", nil).expect(http.StatusBadRequest)
		anonymous().call(t, http.MethodGet, "/programs/"+p.ID+"/applications", nil).expect(http.StatusUnauthorized)

		var termApps struct {
			Data []application `json:"data"`
		}
		admin.mustJSON(t, http.MethodGet, applicationsPath(p)+"?role=mentee", nil, http.StatusOK, &termApps)
		if len(termApps.Data) != 1 {
			t.Fatalf("term applications: %+v", termApps)
		}
		admin.call(t, http.MethodGet, applicationsPath(p)+"?offset=x", nil).expect(http.StatusBadRequest)
		other := publishProgram(t, admin, "Unrelated Program")
		admin.call(t, http.MethodGet, "/programs/"+other.ID+"/terms/"+p.TermID+"/applications", nil).expect(http.StatusNotFound)

		var termTasks struct {
			Data []task `json:"data"`
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/terms/"+p.TermID+"/tasks?status=complete", nil, http.StatusOK, &termTasks)
		if len(termTasks.Data) != 1 {
			t.Fatalf("term tasks: %+v", termTasks)
		}
		admin.call(t, http.MethodGet, "/programs/"+other.ID+"/terms/"+p.TermID+"/tasks", nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodGet, "/programs/"+p.ID+"/terms/"+p.TermID+"/tasks?limit=x", nil).expect(http.StatusBadRequest)

		res := admin.call(t, http.MethodGet, applicationsPath(p)+"/export?tasks_submitted=true", nil).expect(http.StatusOK)
		if ct := res.Header.Get("Content-Type"); ct != "text/csv" {
			t.Fatalf("export content type %q", ct)
		}
		records, err := csv.NewReader(strings.NewReader(string(res.Body))).ReadAll()
		if err != nil || len(records) != 2 || records[1][0] != app.ID {
			t.Fatalf("export: %v %q", err, res.Body)
		}
		res = admin.call(t, http.MethodGet, applicationsPath(p)+"/export?tasks_submitted=false", nil).expect(http.StatusOK)
		if records, _ := csv.NewReader(strings.NewReader(string(res.Body))).ReadAll(); len(records) != 1 {
			t.Fatalf("filtered export: %q", res.Body)
		}
		admin.call(t, http.MethodGet, "/programs/"+other.ID+"/terms/"+p.TermID+"/applications/export", nil).expect(http.StatusNotFound)
	})

	t.Run("acceptance", func(t *testing.T) {
		admin.call(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "accepted"}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "accepted", "attendance_type": "weekends"}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "bogus"}).expect(http.StatusBadRequest)
		admin.call(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "graduated"}).expect(http.StatusConflict)
		admin.mustJSON(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "accepted", "attendance_type": "part_time"}, http.StatusOK, &app)
		if app.Status != "accepted" || app.AttendanceType == nil || *app.AttendanceType != "part_time" {
			t.Fatalf("accepted: %+v", app)
		}
		awaitEmail(t, mentee.Email, "Congratulations", 1)
		awaitEmail(t, admin.Email, "has been accepted", 1)
		hr := awaitEmail(t, e2eHRInbox, "has been accepted", 1)
		if !strings.Contains(hr.Text, "Kenya") || !strings.Contains(hr.Text, mentee.Email) {
			t.Fatalf("HR notice lacks the country or mentee email: %s", hr.Text)
		}
		// An accepted program cannot be hidden.
		admin.call(t, http.MethodPost, "/programs/"+p.ID+"/hide", nil).expect(http.StatusConflict)
	})

	t.Run("custom tasks on an accepted application", func(t *testing.T) {
		var custom task
		mentor.mustJSON(t, http.MethodPost, "/applications/"+app.ID+"/tasks", map[string]any{
			"assignee_id": mentee.ID, "name": "Weekly report", "category": "non_prerequisite", "custom": true, "due_date": day(30),
		}, http.StatusCreated, &custom)
		if custom.Status != "incomplete" || !custom.Custom {
			t.Fatalf("custom task: %+v", custom)
		}
		mentor.call(t, http.MethodPost, "/applications/"+app.ID+"/tasks", map[string]any{"name": "x"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPost, "/applications/"+app.ID+"/tasks", map[string]any{"assignee_id": mentor.ID}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPost, "/applications/"+app.ID+"/tasks", map[string]any{"assignee_id": mentee.ID, "status": "bogus"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPost, "/applications/"+app.ID+"/tasks", map[string]any{"assignee_id": mentee.ID, "category": "bogus"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPost, "/applications/"+app.ID+"/tasks", map[string]any{"assignee_id": mentee.ID, "due_date": "tomorrow"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPost, "/applications/00000000-0000-4000-8000-000000000000/tasks", map[string]any{"assignee_id": mentee.ID}).expect(http.StatusNotFound)

		var edited task
		mentor.mustJSON(t, http.MethodPatch, "/tasks/"+custom.ID, map[string]any{"name": "Weekly status report", "status": "submitted", "due_date": ""}, http.StatusOK, &edited)
		if edited.Name != "Weekly status report" || edited.Status != "submitted" || edited.DueDate != nil {
			t.Fatalf("edited: %+v", edited)
		}
		mentee.call(t, http.MethodPatch, "/tasks/"+custom.ID, map[string]any{"name": "x"}).expect(http.StatusForbidden)
		mentor.call(t, http.MethodPatch, "/tasks/"+custom.ID, map[string]any{"file": "x"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPatch, "/tasks/"+custom.ID, map[string]any{"application_status": "accepted"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPatch, "/tasks/"+custom.ID, map[string]any{"category": "bogus"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPatch, "/tasks/"+custom.ID, map[string]any{"due_date": "soon"}).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodPatch, "/tasks/00000000-0000-4000-8000-000000000000", map[string]any{"name": "x"}).expect(http.StatusNotFound)

		var got task
		mentee.mustJSON(t, http.MethodGet, "/tasks/"+custom.ID, nil, http.StatusOK, &got)
		mentor.mustJSON(t, http.MethodGet, "/tasks/"+custom.ID, nil, http.StatusOK, &got)
		mentor.call(t, http.MethodGet, "/tasks/00000000-0000-4000-8000-000000000000", nil).expect(http.StatusNotFound)
		if n := len(tasksOf(t, mentor, app.ID)); n != 2 {
			t.Fatalf("application has %d tasks; want 2", n)
		}
		mentor.call(t, http.MethodGet, "/applications/"+app.ID+"/tasks?limit=x", nil).expect(http.StatusBadRequest)
		mentor.call(t, http.MethodGet, "/applications/00000000-0000-4000-8000-000000000000/tasks", nil).expect(http.StatusNotFound)

		mentee.call(t, http.MethodDelete, "/tasks/"+custom.ID, nil).expect(http.StatusForbidden)
		mentor.call(t, http.MethodDelete, "/tasks/"+custom.ID, nil).expect(http.StatusNoContent)
		mentor.call(t, http.MethodDelete, "/tasks/"+custom.ID, nil).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodDelete, "/tasks/"+prereq.ID, nil).expect(http.StatusUnauthorized)
	})

	t.Run("graduation", func(t *testing.T) {
		admin.mustJSON(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "graduated"}, http.StatusOK, &app)
		if app.Status != "graduated" {
			t.Fatalf("graduated: %+v", app)
		}
		admin.call(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "accepted"}).expect(http.StatusConflict)
		var past struct {
			Data []application `json:"data"`
		}
		admin.mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/terms/"+p.TermID+"/past-mentees", nil, http.StatusOK, &past)
		if len(past.Data) != 1 || past.Data[0].ID != app.ID {
			t.Fatalf("past mentees: %+v", past)
		}
		other := publishProgram(t, admin, "Other Graduation Program")
		admin.call(t, http.MethodGet, "/programs/"+other.ID+"/terms/"+p.TermID+"/past-mentees", nil).expect(http.StatusNotFound)
		var mentees struct {
			Data []any `json:"data"`
		}
		anonymous().mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/mentees", nil, http.StatusOK, &mentees)
		if len(mentees.Data) != 1 {
			t.Fatalf("catalog mentees: %+v", mentees)
		}
	})

	t.Run("deleting the application removes its tasks", func(t *testing.T) {
		admin.call(t, http.MethodDelete, "/applications/"+app.ID, nil).expect(http.StatusNoContent)
		admin.call(t, http.MethodGet, "/applications/"+app.ID, nil).expect(http.StatusNotFound)
		admin.call(t, http.MethodDelete, "/applications/"+app.ID, nil).expect(http.StatusNotFound)
		if n := dbCount(t, "SELECT count(*) FROM tasks WHERE application_id = $1", app.ID); n != 0 {
			t.Fatalf("%d tasks left", n)
		}
	})
}

func TestE2EMentorApplication(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Mentor Application Program")
	volunteer := signIn(t, "mentor-applicant")

	var app application
	volunteer.mustJSON(t, http.MethodPost, applicationsPath(p), map[string]any{"role": "mentor"}, http.StatusCreated, &app)
	if app.Role != "mentor" {
		t.Fatalf("application: %+v", app)
	}
	// The same person may hold a mentee application for the same term alongside it.
	apply(t, volunteer, p)
	admin.mustJSON(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "accepted", "attendance_type": "full_time"}, http.StatusOK, &app)

	var roster struct {
		Data []member `json:"data"`
	}
	anonymous().mustJSON(t, http.MethodGet, "/programs/"+p.ID+"/members", nil, http.StatusOK, &roster)
	if len(roster.Data) != 1 || roster.Data[0].UserID != volunteer.ID || roster.Data[0].Status != "active" {
		t.Fatalf("accepted mentor is not on the roster: %+v", roster)
	}
	awaitFGA(t, "lfx.fga-sync.member_put", func(m fgaMessage) bool {
		return m.ObjectType == "mentorship_program" && m.Data.UID == p.ID && m.Data.Username == "mentor-applicant"
	})
	// Accepting a mentor sends no mentee welcome.
	if n := len(e2e.fakes.emailsTo(volunteer.Email)); n != 0 {
		t.Fatalf("mentor applicant got %d emails", n)
	}
}

func TestE2EApplicationGuards(t *testing.T) {
	reset(t)
	admin := signIn(t, "admin")
	p := publishProgram(t, admin, "Guarded Program")
	mentee := signIn(t, "mentee")

	t.Run("create validation", func(t *testing.T) {
		mentee.call(t, http.MethodPost, applicationsPath(p), map[string]any{"role": "observer"}).expect(http.StatusBadRequest)
		other := publishProgram(t, admin, "Other Guarded Program")
		mentee.call(t, http.MethodPost, "/programs/"+other.ID+"/terms/"+p.TermID+"/applications", map[string]any{"role": "mentee"}).expect(http.StatusNotFound)
		mentee.call(t, http.MethodPost, "/programs/"+p.ID+"/terms/00000000-0000-4000-8000-000000000000/applications", map[string]any{"role": "mentee"}).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodPost, applicationsPath(p), map[string]any{"role": "mentee"}).expect(http.StatusUnauthorized)

		var future term
		admin.mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/terms", termBody("Future", 30, 40, 50, 90), http.StatusCreated, &future)
		res := mentee.call(t, http.MethodPost, "/programs/"+p.ID+"/terms/"+future.ID+"/applications", map[string]any{"role": "mentee"}).expect(http.StatusUnprocessableEntity)
		if !strings.Contains(res.errorMessage(), "not opened yet") {
			t.Fatalf("got %q", res.errorMessage())
		}
		var past term
		admin.mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/terms", termBody("Past window", -10, -5, 10, 90), http.StatusCreated, &past)
		mentee.call(t, http.MethodPost, "/programs/"+p.ID+"/terms/"+past.ID+"/applications", map[string]any{"role": "mentee"}).expect(http.StatusUnprocessableEntity)
		admin.mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/terms/"+future.ID+"/close", nil, http.StatusOK, nil)
		mentee.call(t, http.MethodPost, "/programs/"+p.ID+"/terms/"+future.ID+"/applications", map[string]any{"role": "mentee"}).expect(http.StatusUnprocessableEntity)
	})

	app := apply(t, mentee, p)
	mentee.call(t, http.MethodPost, applicationsPath(p), map[string]any{"role": "mentee"}).expect(http.StatusConflict)

	t.Run("only the applicant withdraws", func(t *testing.T) {
		admin.call(t, http.MethodPost, "/applications/"+app.ID+"/withdraw", nil).expect(http.StatusForbidden)
		admin.call(t, http.MethodPatch, "/applications/"+app.ID+"/status", map[string]any{"status": "withdrawn"}).expect(http.StatusForbidden)
		admin.call(t, http.MethodPost, "/applications/"+app.ID+"/reapply", nil).expect(http.StatusForbidden)
	})

	t.Run("withdraw and reapply up to the limit", func(t *testing.T) {
		current := app
		for i := 1; i <= 3; i++ {
			var withdrawn application
			mentee.mustJSON(t, http.MethodPost, "/applications/"+current.ID+"/withdraw", nil, http.StatusOK, &withdrawn)
			if withdrawn.Status != "withdrawn" {
				t.Fatalf("withdraw %d: %+v", i, withdrawn)
			}
			mentee.call(t, http.MethodPost, "/applications/"+current.ID+"/withdraw", nil).expect(http.StatusConflict)
			if i == 3 {
				res := mentee.call(t, http.MethodPost, "/applications/"+current.ID+"/reapply", nil).expect(http.StatusUnprocessableEntity)
				if !strings.Contains(res.errorMessage(), "reapplication limit") {
					t.Fatalf("got %q", res.errorMessage())
				}
				break
			}
			var again application
			mentee.mustJSON(t, http.MethodPost, "/applications/"+current.ID+"/reapply", nil, http.StatusCreated, &again)
			if again.Status != "pending" {
				t.Fatalf("reapply %d: %+v", i, again)
			}
			if n := len(tasksOf(t, mentee, again.ID)); n != 1 {
				t.Fatalf("reapplication has %d prerequisite tasks", n)
			}
			current = again
		}
		mentee.call(t, http.MethodPost, "/applications/00000000-0000-4000-8000-000000000000/reapply", nil).expect(http.StatusNotFound)
	})

	t.Run("a declined applicant cannot reapply", func(t *testing.T) {
		other := signIn(t, "declined-mentee")
		declined := apply(t, other, p)
		admin.mustJSON(t, http.MethodPatch, "/applications/"+declined.ID+"/status", map[string]any{"status": "hold"}, http.StatusOK, nil)
		admin.mustJSON(t, http.MethodPatch, "/applications/"+declined.ID+"/status", map[string]any{"status": "declined"}, http.StatusOK, nil)
		res := other.call(t, http.MethodPost, "/applications/"+declined.ID+"/reapply", nil).expect(http.StatusConflict)
		if !strings.Contains(res.errorMessage(), "conflict") {
			t.Fatalf("got %q", res.errorMessage())
		}
		// An admin can reopen it.
		admin.mustJSON(t, http.MethodPatch, "/applications/"+declined.ID+"/status", map[string]any{"status": "pending"}, http.StatusOK, nil)
	})

	t.Run("assisted withdrawal", func(t *testing.T) {
		helped := signIn(t, "helped-mentee")
		a := apply(t, helped, p)
		var withdrawn application
		admin.mustJSON(t, http.MethodPost, "/applications/"+a.ID+"/withdraw-for-mentee", nil, http.StatusOK, &withdrawn)
		if withdrawn.Status != "withdrawn" {
			t.Fatalf("assisted withdrawal: %+v", withdrawn)
		}
		admin.call(t, http.MethodPost, "/applications/00000000-0000-4000-8000-000000000000/withdraw-for-mentee", nil).expect(http.StatusNotFound)
		anonymous().call(t, http.MethodPost, "/applications/"+a.ID+"/withdraw-for-mentee", nil).expect(http.StatusUnauthorized)
	})

	t.Run("bulk decline and closing a term decline pending applications", func(t *testing.T) {
		for _, lfid := range []string{"bulk-a", "bulk-b"} {
			apply(t, signIn(t, lfid), p)
		}
		var res struct {
			DeclinedCount int `json:"declined_count"`
		}
		admin.mustJSON(t, http.MethodPost, applicationsPath(p)+"/bulk-decline", nil, http.StatusOK, &res)
		if res.DeclinedCount < 2 {
			t.Fatalf("bulk decline: %+v", res)
		}
		other := publishProgram(t, admin, "Bulk Other Program")
		admin.call(t, http.MethodPost, "/programs/"+other.ID+"/terms/"+p.TermID+"/applications/bulk-decline", nil).expect(http.StatusNotFound)

		late := apply(t, signIn(t, "bulk-c"), p)
		admin.mustJSON(t, http.MethodPost, "/programs/"+p.ID+"/terms/"+p.TermID+"/close", nil, http.StatusOK, nil)
		var got application
		admin.mustJSON(t, http.MethodGet, "/applications/"+late.ID, nil, http.StatusOK, &got)
		if got.Status != "declined" {
			t.Fatalf("closing the term left %+v", got)
		}
	})
}
