// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// Enrollment maps to the public.enrollments table.
type Enrollment struct {
	ID                string     `json:"id"`
	ProgramTermID     string     `json:"program_term_id"`
	MenteeUserID      string     `json:"mentee_user_id"`
	Status            string     `json:"status"` // active | graduated | withdrawn | hold
	ProgramTermStatus *string    `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time `json:"end_date_time,omitempty"`
	TasksSubmitted    bool       `json:"tasks_submitted"`
	AdminNotified     bool       `json:"admin_notified"`
	CreatedOn         time.Time  `json:"created_on"`
	UpdatedOn         time.Time  `json:"updated_on"`
}

// EnrollmentCreateInput is the request body for creating an enrollment.
type EnrollmentCreateInput struct {
	ID                string     `json:"id"`
	MenteeUserID      string     `json:"mentee_user_id"`
	Status            string     `json:"status"`
	ProgramTermStatus *string    `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time `json:"end_date_time,omitempty"`
}

// EnrollmentUpdateInput is the request body for updating an enrollment.
type EnrollmentUpdateInput struct {
	Status            *string    `json:"status,omitempty"`
	ProgramTermStatus *string    `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time `json:"end_date_time,omitempty"`
	TasksSubmitted    *bool      `json:"tasks_submitted,omitempty"`
	AdminNotified     *bool      `json:"admin_notified,omitempty"`
}
