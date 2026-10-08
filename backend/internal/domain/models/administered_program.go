// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// AdministeredProgramStatus is the status a program admin sees on their
// programs list. It groups ProgramStatus with the program's terms: a published
// program with an open term, or with no terms yet, is open; one whose
// non-deleted terms are all closed is completed.
type AdministeredProgramStatus string

const (
	AdministeredProgramStatusOpen          AdministeredProgramStatus = "open"
	AdministeredProgramStatusPendingReview AdministeredProgramStatus = "pending_review"
	AdministeredProgramStatusCompleted     AdministeredProgramStatus = "completed"
	AdministeredProgramStatusRejected      AdministeredProgramStatus = "rejected"
	AdministeredProgramStatusHidden        AdministeredProgramStatus = "hidden"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s AdministeredProgramStatus) IsValid() bool {
	switch s {
	case AdministeredProgramStatusOpen, AdministeredProgramStatusPendingReview, AdministeredProgramStatusCompleted,
		AdministeredProgramStatusRejected, AdministeredProgramStatusHidden:
		return true
	}
	return false
}

// AdministeredProgram is one row of the caller's programs list: a program the
// caller is an active program admin of, with its current term and counts.
type AdministeredProgram struct {
	ID          string                    `json:"id"`
	Slug        string                    `json:"slug"`
	Name        string                    `json:"name"`
	ProjectUID  *string                   `json:"project_uid,omitempty"`
	ProjectName *string                   `json:"project_name,omitempty"`
	LogoURL     *string                   `json:"logo_url,omitempty"`
	Status      ProgramStatus             `json:"status"`
	AdminStatus AdministeredProgramStatus `json:"admin_status"`
	// Term is the started open term, else the next open term, else the latest
	// closed term — the same choice as MentoredProgram.Term.
	Term      *ProgramTerm       `json:"term,omitempty"`
	Stats     ProgramHeaderStats `json:"stats"`
	CreatedOn time.Time          `json:"created_on"`
	UpdatedOn time.Time          `json:"updated_on"`
}
