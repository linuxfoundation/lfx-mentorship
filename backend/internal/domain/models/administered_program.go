// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// AdministeredProgramStatus is the status a program admin sees on their
// programs list. It groups ProgramStatus with the program's terms, ignoring
// deleted ones. A published program is, in order of precedence:
//   - completed when it has a closed term and no open one
//   - accepting when an open term is inside its application window
//   - in_progress when an open term is inside its term window
//   - upcoming when an open term's application window has not opened, or it has no terms yet
//   - in_progress otherwise (an open term between or past its windows, or without dates)
type AdministeredProgramStatus string

const (
	AdministeredProgramStatusAccepting     AdministeredProgramStatus = "accepting"
	AdministeredProgramStatusInProgress    AdministeredProgramStatus = "in_progress"
	AdministeredProgramStatusUpcoming      AdministeredProgramStatus = "upcoming"
	AdministeredProgramStatusPendingReview AdministeredProgramStatus = "pending_review"
	AdministeredProgramStatusCompleted     AdministeredProgramStatus = "completed"
	AdministeredProgramStatusRejected      AdministeredProgramStatus = "rejected"
	AdministeredProgramStatusHidden        AdministeredProgramStatus = "hidden"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s AdministeredProgramStatus) IsValid() bool {
	switch s {
	case AdministeredProgramStatusAccepting, AdministeredProgramStatusInProgress, AdministeredProgramStatusUpcoming,
		AdministeredProgramStatusPendingReview, AdministeredProgramStatusCompleted,
		AdministeredProgramStatusRejected, AdministeredProgramStatusHidden:
		return true
	}
	return false
}

// AdministeredProgram is one row of the caller's programs list: a program the
// caller is an active program admin of, with its program-wide counts.
type AdministeredProgram struct {
	ID          string                    `json:"id"`
	Slug        string                    `json:"slug"`
	Name        string                    `json:"name"`
	ProjectUID  *string                   `json:"project_uid,omitempty"`
	ProjectName *string                   `json:"project_name,omitempty"`
	LogoURL     *string                   `json:"logo_url,omitempty"`
	Status      ProgramStatus             `json:"status"`
	AdminStatus AdministeredProgramStatus `json:"admin_status"`
	Stats       ProgramHeaderStats        `json:"stats"`
	CreatedOn   time.Time                 `json:"created_on"`
	UpdatedOn   time.Time                 `json:"updated_on"`
}
