// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// TaskStatus enumerates valid values for tasks.status.
type TaskStatus string

const (
	TaskStatusIncomplete TaskStatus = "incomplete"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusComplete   TaskStatus = "complete"
	TaskStatusSubmitted  TaskStatus = "submitted"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusIncomplete, TaskStatusInProgress, TaskStatusComplete, TaskStatusSubmitted:
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
	File              *string            `json:"file,omitempty"`
	DueDate           *string            `json:"due_date,omitempty"` // ISO date string
	CreatedBy         *string            `json:"created_by,omitempty"`
	CreatedOn         time.Time          `json:"created_on"`
	UpdatedOn         time.Time          `json:"updated_on"`
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

// TaskUpdateInput is the request body for updating a task.
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
