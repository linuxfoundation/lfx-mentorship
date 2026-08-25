// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// Application maps to the public.applications table.
type Application struct {
	ID                string     `json:"id"`
	ProgramTermID     string     `json:"program_term_id"`
	UserID            string     `json:"user_id"`
	Role              string     `json:"role"`   // mentor | mentee
	Status            string     `json:"status"` // pending | accepted | active | declined | withdrawn | graduated | hold
	ProgramTermStatus *string    `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time `json:"end_date_time,omitempty"`
	AttendanceType    *string    `json:"attendance_type,omitempty"` // full_time | part_time (set on accept)
	TasksSubmitted    bool       `json:"tasks_submitted"`
	AdminNotified     bool       `json:"admin_notified"`
	CreatedOn         time.Time  `json:"created_on"`
	UpdatedOn         time.Time  `json:"updated_on"`
}

// ApplicationCreateInput is the request body for submitting an application.
// Status is intentionally absent — the service always assigns "pending".
type ApplicationCreateInput struct {
	ID                string     `json:"id"`
	UserID            string     `json:"user_id"`
	Role              string     `json:"role"`
	Status            string     `json:"-"` // server-assigned; never read from client
	ProgramTermStatus *string    `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time `json:"end_date_time,omitempty"`
	AttendanceType    *string    `json:"attendance_type,omitempty"`
}

// ApplicationUpdateInput is the request body for updating an application's status.
type ApplicationUpdateInput struct {
	Status            *string    `json:"status,omitempty"`
	ProgramTermStatus *string    `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time `json:"end_date_time,omitempty"`
	AttendanceType    *string    `json:"attendance_type,omitempty"`
	TasksSubmitted    *bool      `json:"tasks_submitted,omitempty"`
	AdminNotified     *bool      `json:"admin_notified,omitempty"`
	// ActorID is the caller's user ID for permission checks; not persisted.
	ActorID string `json:"-"`
}
