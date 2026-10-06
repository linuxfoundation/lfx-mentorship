// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"encoding/json"
	"time"
)

// TaskStatus enumerates valid values for tasks.status.
type TaskStatus string

const (
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusSubmitted  TaskStatus = "submitted"

	TaskStatusIncomplete TaskStatus = "incomplete"
	TaskStatusComplete   TaskStatus = "complete"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusIncomplete, TaskStatusInProgress, TaskStatusSubmitted, TaskStatusComplete:
		return true
	}
	return false
}

// TaskCategory enumerates valid values for tasks.category.
type TaskCategory string

const (
	TaskCategoryPrerequisite    TaskCategory = "prerequisite"
	TaskCategoryNonPrerequisite TaskCategory = "non_prerequisite"
)

// IsValid reports whether the category value is one of the allowed enum members.
func (c TaskCategory) IsValid() bool {
	switch c {
	case TaskCategoryPrerequisite, TaskCategoryNonPrerequisite:
		return true
	}
	return false
}

// Task maps to the public.tasks table.
type Task struct {
	ID                string             `json:"id"`
	ApplicationID     *string            `json:"application_id,omitempty"`
	ProgramTermID     *string            `json:"program_term_id,omitempty"`
	AssigneeID        string             `json:"assignee_id"`
	OwnerID           *string            `json:"owner_id,omitempty"`
	Name              *string            `json:"name,omitempty"`
	Description       *string            `json:"description,omitempty"`
	Category          *TaskCategory      `json:"category,omitempty"`
	Status            TaskStatus         `json:"status"`
	ApplicationStatus *ApplicationStatus `json:"application_status,omitempty"` // denormalised from applications.status
	ProgramTermStatus *ProgramTermStatus `json:"program_term_status,omitempty"`
	Custom            bool               `json:"custom"`
	SubmitFile        *string            `json:"submit_file,omitempty"`
	// File is the private object key of the submission; responses carry the download route instead.
	File      *string   `json:"file,omitempty"`
	DueDate   *string   `json:"due_date,omitempty"` // ISO date string
	CreatedBy *string   `json:"created_by,omitempty"`
	CreatedOn time.Time `json:"created_on"`
	UpdatedOn time.Time `json:"updated_on"`
}

// TaskFileDownloadPath is the API route that serves a task's submission.
func TaskFileDownloadPath(taskID string) string {
	return "/mentorship/v1/tasks/" + taskID + "/file-download"
}

// MarshalJSON replaces the private object key in file with the download route.
func (t Task) MarshalJSON() ([]byte, error) {
	type task Task
	out := struct {
		task
		File *string `json:"file,omitempty"`
	}{task: task(t)}
	if t.File != nil && *t.File != "" {
		route := TaskFileDownloadPath(t.ID)
		out.File = &route
	}
	return json.Marshal(out)
}

// TaskCreateInput is the request body for creating a task.
type TaskCreateInput struct {
	ID            string        `json:"id"`
	ProgramTermID *string       `json:"program_term_id,omitempty"`
	AssigneeID    string        `json:"assignee_id"`
	OwnerID       *string       `json:"owner_id,omitempty"`
	Name          *string       `json:"name,omitempty"`
	Description   *string       `json:"description,omitempty"`
	Category      *TaskCategory `json:"category,omitempty"`
	Status        TaskStatus    `json:"status"`
	Custom        bool          `json:"custom"`
	SubmitFile    *string       `json:"submit_file,omitempty"`
	DueDate       *string       `json:"due_date,omitempty"`
	CreatedBy     *string       `json:"created_by,omitempty"`
}

// TaskUpdateInput is the request body for updating a task. A nil field is left unchanged; an
// empty SubmitFile or DueDate clears it.
type TaskUpdateInput struct {
	Name              *string            `json:"name,omitempty"`
	Description       *string            `json:"description,omitempty"`
	Category          *TaskCategory      `json:"category,omitempty"`
	Status            *TaskStatus        `json:"status,omitempty"`
	ApplicationStatus *ApplicationStatus `json:"application_status,omitempty"`
	ProgramTermStatus *ProgramTermStatus `json:"program_term_status,omitempty"`
	Custom            *bool              `json:"custom,omitempty"`
	SubmitFile        *string            `json:"submit_file,omitempty"`
	File              *string            `json:"file,omitempty"`
	DueDate           *string            `json:"due_date,omitempty"`
	// ActorID is the caller's user ID used for permission checks; not persisted.
	ActorID string `json:"-"`
}
