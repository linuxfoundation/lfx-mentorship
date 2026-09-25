// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestNewApplicationIndexConfig(t *testing.T) {
	config := NewApplicationIndexConfig("app-1", "program-1", "user-1", "mentee", "accepted")

	if got, want := config["object_ref"], "mentorship_application:app-1"; got != want {
		t.Fatalf("object_ref = %v, want %v", got, want)
	}
	if got, want := config["access_check_relation"], "auditor"; got != want {
		t.Fatalf("access_check_relation = %v, want %v", got, want)
	}
	if got, want := config["history_check_object"], "mentorship_program:program-1"; got != want {
		t.Fatalf("history_check_object = %v, want %v", got, want)
	}
}

func TestNewApplicationIndexDocumentIncludesLifecycleFields(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	attendance := models.AttendanceTypePartTime
	application := &models.Application{
		ID:             "app-1",
		ProgramTermID:  "term-1",
		UserID:         "user-1",
		Role:           models.ApplicationRoleMentee,
		Status:         models.ApplicationStatusAccepted,
		StartDateTime:  &start,
		EndDateTime:    &end,
		AttendanceType: &attendance,
		TasksSubmitted: true,
	}

	document := NewApplicationIndexDocument(application)
	if document.StartDateTime == nil || !document.StartDateTime.Equal(start) || document.EndDateTime == nil || !document.EndDateTime.Equal(end) {
		t.Fatalf("lifecycle dates = %#v, want %s to %s", document, start, end)
	}
	if document.AttendanceType == nil || *document.AttendanceType != attendance || !document.TasksSubmitted {
		t.Fatalf("lifecycle state = %#v, want attendance=%s tasks_submitted=true", document, attendance)
	}
}

func TestNewTaskIndexDocument(t *testing.T) {
	createdOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	name := "First task"
	description := "Upload the latest resume."
	category := models.TaskCategoryPrerequisite
	submitFile := "required"
	dueDate := "2026-02-15"
	task := &models.Task{
		ID:            "task-1",
		ApplicationID: stringPointer("app-1"),
		AssigneeID:    "user-1",
		Name:          &name,
		Description:   &description,
		Category:      &category,
		Status:        models.TaskStatusComplete,
		Custom:        true,
		SubmitFile:    &submitFile,
		DueDate:       &dueDate,
		CreatedOn:     createdOn,
		UpdatedOn:     createdOn,
	}

	document := NewTaskIndexDocument(task)
	if document.ID != task.ID || document.ApplicationID == nil || *document.ApplicationID != "app-1" {
		t.Fatalf("document identity = %#v, want task-1/app-1", document)
	}
	if document.Name == nil || *document.Name != name || document.Status != models.TaskStatusComplete {
		t.Fatalf("document fields = %#v, want task fields", document)
	}
	if !document.Prerequisite || !document.Custom || document.Description == nil || *document.Description != description || document.SubmitFile == nil || *document.SubmitFile != submitFile || document.DueDate == nil || *document.DueDate != dueDate {
		t.Fatalf("document contract fields = %#v, want UI task fields", document)
	}
}

func stringPointer(value string) *string {
	return &value
}
