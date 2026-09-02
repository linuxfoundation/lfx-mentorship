// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// ApplicationStatus enumerates valid values for applications.status.
// "declined" is the shared vocabulary for a turned-down application of either
// role — "rejected" belongs to ProgramStatus (program moderation) and must not
// be reused here.
type ApplicationStatus string

const (
	ApplicationStatusPending   ApplicationStatus = "pending"
	ApplicationStatusAccepted  ApplicationStatus = "accepted"
	ApplicationStatusActive    ApplicationStatus = "active"
	ApplicationStatusDeclined  ApplicationStatus = "declined"
	ApplicationStatusWithdrawn ApplicationStatus = "withdrawn"
	ApplicationStatusGraduated ApplicationStatus = "graduated"
	ApplicationStatusHold      ApplicationStatus = "hold"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s ApplicationStatus) IsValid() bool {
	switch s {
	case ApplicationStatusPending, ApplicationStatusAccepted, ApplicationStatusActive,
		ApplicationStatusDeclined, ApplicationStatusWithdrawn, ApplicationStatusGraduated,
		ApplicationStatusHold:
		return true
	}
	return false
}

// ApplicationRole enumerates valid values for applications.role.
type ApplicationRole string

const (
	ApplicationRoleMentor ApplicationRole = "mentor"
	ApplicationRoleMentee ApplicationRole = "mentee"
)

// IsValid reports whether the role value is one of the allowed enum members.
func (r ApplicationRole) IsValid() bool {
	switch r {
	case ApplicationRoleMentor, ApplicationRoleMentee:
		return true
	}
	return false
}

// AttendanceType enumerates valid values for applications.attendance_type.
type AttendanceType string

const (
	AttendanceTypeFullTime AttendanceType = "full_time"
	AttendanceTypePartTime AttendanceType = "part_time"
)

// IsValid reports whether the attendance type is one of the allowed enum members.
func (a AttendanceType) IsValid() bool {
	switch a {
	case AttendanceTypeFullTime, AttendanceTypePartTime:
		return true
	}
	return false
}

// Application maps to the public.applications table.
type Application struct {
	ID                string             `json:"id"`
	ProgramTermID     string             `json:"program_term_id"`
	UserID            string             `json:"user_id"`
	Role              ApplicationRole    `json:"role"`
	Status            ApplicationStatus  `json:"status"`
	ProgramTermStatus *ProgramTermStatus `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time         `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time         `json:"end_date_time,omitempty"`
	AttendanceType    *AttendanceType    `json:"attendance_type,omitempty"` // set on accept
	TasksSubmitted    bool               `json:"tasks_submitted"`
	AdminNotified     bool               `json:"admin_notified"`
	CreatedOn         time.Time          `json:"created_on"`
	UpdatedOn         time.Time          `json:"updated_on"`
}

// ApplicationCreateInput is the request body for submitting an application.
// Status is intentionally absent — the service always assigns "pending".
type ApplicationCreateInput struct {
	ID                string             `json:"id"`
	UserID            string             `json:"user_id"`
	Role              ApplicationRole    `json:"role"`
	Status            ApplicationStatus  `json:"-"` // server-assigned; never read from client
	ProgramTermStatus *ProgramTermStatus `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time         `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time         `json:"end_date_time,omitempty"`
	AttendanceType    *AttendanceType    `json:"attendance_type,omitempty"`
}

// ApplicationUpdateInput is the request body for updating an application's status.
type ApplicationUpdateInput struct {
	Status            *ApplicationStatus `json:"status,omitempty"`
	ProgramTermStatus *ProgramTermStatus `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time         `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time         `json:"end_date_time,omitempty"`
	AttendanceType    *AttendanceType    `json:"attendance_type,omitempty"`
	TasksSubmitted    *bool              `json:"tasks_submitted,omitempty"`
	AdminNotified     *bool              `json:"admin_notified,omitempty"`
	// ActorID is the caller's user ID for permission checks; not persisted.
	ActorID string `json:"-"`
}
