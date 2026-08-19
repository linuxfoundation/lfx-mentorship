// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// Application maps to the public.applications table.
type Application struct {
	ID                string    `json:"id"`
	ProgramTermID     string    `json:"program_term_id"`
	UserID            string    `json:"user_id"`
	Role              string    `json:"role"`   // mentor | mentee
	Status            string    `json:"status"` // pending | accepted | declined | withdrawn
	ProgramTermStatus *string   `json:"program_term_status,omitempty"`
	TasksSubmitted    bool      `json:"tasks_submitted"`
	AdminNotified     bool      `json:"admin_notified"`
	CreatedOn         time.Time `json:"created_on"`
	UpdatedOn         time.Time `json:"updated_on"`
}

// ApplicationCreateInput is the request body for submitting an application.
type ApplicationCreateInput struct {
	ID                string  `json:"id"`
	UserID            string  `json:"user_id"`
	Role              string  `json:"role"`
	Status            string  `json:"status"`
	ProgramTermStatus *string `json:"program_term_status,omitempty"`
}

// ApplicationUpdateInput is the request body for updating an application's status.
type ApplicationUpdateInput struct {
	Status            *string `json:"status,omitempty"`
	ProgramTermStatus *string `json:"program_term_status,omitempty"`
	TasksSubmitted    *bool   `json:"tasks_submitted,omitempty"`
	AdminNotified     *bool   `json:"admin_notified,omitempty"`
}
