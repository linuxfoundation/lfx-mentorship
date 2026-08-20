// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// Task maps to the public.tasks table.
type Task struct {
	ID                string    `json:"id"`
	ApplicationID     *string   `json:"application_id,omitempty"`
	ProgramTermID     *string   `json:"program_term_id,omitempty"`
	AssigneeID        string    `json:"assignee_id"`
	OwnerID           *string   `json:"owner_id,omitempty"`
	Name              *string   `json:"name,omitempty"`
	Description       *string   `json:"description,omitempty"`
	Category          *string   `json:"category,omitempty"` // prerequisite | non_prerequisite
	Status            string    `json:"status"`             // incomplete | in_progress | complete | submitted
	ApplicationStatus *string   `json:"application_status,omitempty"`
	ProgramTermStatus *string   `json:"program_term_status,omitempty"`
	Custom            bool      `json:"custom"`
	SubmitFile        *string   `json:"submit_file,omitempty"`
	File              *string   `json:"file,omitempty"`
	DueDate           *string   `json:"due_date,omitempty"` // ISO date string
	CreatedBy         *string   `json:"created_by,omitempty"`
	CreatedOn         time.Time `json:"created_on"`
	UpdatedOn         time.Time `json:"updated_on"`
}

// TaskCreateInput is the request body for creating a task.
type TaskCreateInput struct {
	ID            string  `json:"id"`
	ProgramTermID *string `json:"program_term_id,omitempty"`
	AssigneeID    string  `json:"assignee_id"`
	OwnerID       *string `json:"owner_id,omitempty"`
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	Category      *string `json:"category,omitempty"`
	Status        string  `json:"status"`
	Custom        bool    `json:"custom"`
	SubmitFile    *string `json:"submit_file,omitempty"`
	DueDate       *string `json:"due_date,omitempty"`
	CreatedBy     *string `json:"created_by,omitempty"`
}

// TaskUpdateInput is the request body for updating a task.
type TaskUpdateInput struct {
	Name              *string `json:"name,omitempty"`
	Description       *string `json:"description,omitempty"`
	Category          *string `json:"category,omitempty"`
	Status            *string `json:"status,omitempty"`
	ApplicationStatus *string `json:"application_status,omitempty"`
	ProgramTermStatus *string `json:"program_term_status,omitempty"`
	Custom            *bool   `json:"custom,omitempty"`
	SubmitFile        *string `json:"submit_file,omitempty"`
	File              *string `json:"file,omitempty"`
	DueDate           *string `json:"due_date,omitempty"`
	// ActorID is the caller's user ID used for permission checks; not persisted.
	ActorID string `json:"-"`
}
