// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

type ProgramTermManagementRow struct {
	ProgramTerm
	Pending   int `json:"pending"`
	Declined  int `json:"declined"`
	Accepted  int `json:"accepted"`
	Graduated int `json:"graduated"`
}

// ProgramTermStatus enumerates valid values for program_terms.status.
type ProgramTermStatus string

const (
	ProgramTermStatusOpen    ProgramTermStatus = "open"
	ProgramTermStatusClosed  ProgramTermStatus = "closed"
	ProgramTermStatusDeleted ProgramTermStatus = "deleted"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s ProgramTermStatus) IsValid() bool {
	switch s {
	case ProgramTermStatusOpen, ProgramTermStatusClosed, ProgramTermStatusDeleted:
		return true
	}
	return false
}

// ProgramTerm maps to the public.program_terms table.
type ProgramTerm struct {
	ID                   string            `json:"id"`
	ProgramID            string            `json:"program_id"`
	Name                 string            `json:"name"`
	Status               ProgramTermStatus `json:"status"`
	ActiveUsers          int               `json:"active_users"`
	StartDateTime        *time.Time        `json:"start_date_time,omitempty"`
	EndDateTime          *time.Time        `json:"end_date_time,omitempty"`
	ApplicationStartDate *time.Time        `json:"application_start_date,omitempty"`
	ApplicationEndDate   *time.Time        `json:"application_end_date,omitempty"`
	CreatedOn            time.Time         `json:"created_on"`
	UpdatedOn            time.Time         `json:"updated_on"`
}

// DiscoveryLabel returns the public-facing term state label per FR-017.
// Mapping: open+window future→"Coming Soon"; open+in window→"Apply Now";
// open+window past→"In Progress"; closed/deleted/nil dates→"Completed".
func (t *ProgramTerm) DiscoveryLabel(now time.Time) string {
	if t.Status != ProgramTermStatusOpen {
		return "Completed"
	}
	if t.ApplicationStartDate == nil || t.ApplicationEndDate == nil {
		return "In Progress"
	}
	if now.Before(*t.ApplicationStartDate) {
		return "Coming Soon"
	}
	if now.After(*t.ApplicationEndDate) {
		return "In Progress"
	}
	return "Apply Now"
}

// AcceptsApplications reports whether the term takes applications at now: it is
// open and now is inside its application window. A missing window date leaves
// that side open. ApplicationService.Create enforces the same rule on submit.
func (t *ProgramTerm) AcceptsApplications(now time.Time) bool {
	if t.Status != ProgramTermStatusOpen {
		return false
	}
	if t.ApplicationStartDate != nil && now.Before(*t.ApplicationStartDate) {
		return false
	}
	if t.ApplicationEndDate != nil && now.After(*t.ApplicationEndDate) {
		return false
	}
	return true
}

// ProgramTermApplyTarget is the header of the mentee apply page: the names the
// page shows and whether the term takes applications now.
type ProgramTermApplyTarget struct {
	ProgramName           string `json:"program_name"`
	ProjectName           string `json:"project_name,omitempty"`
	TermName              string `json:"term_name"`
	AcceptingApplications bool   `json:"accepting_applications"`
}

// ProgramTermCreateInput is the request body for creating a program term.
type ProgramTermCreateInput struct {
	ID                   string            `json:"id"`
	ProgramID            string            `json:"program_id"`
	Name                 string            `json:"name"`
	Status               ProgramTermStatus `json:"status"`
	ActiveUsers          int               `json:"active_users"`
	StartDateTime        *time.Time        `json:"start_date_time,omitempty"`
	EndDateTime          *time.Time        `json:"end_date_time,omitempty"`
	ApplicationStartDate *time.Time        `json:"application_start_date,omitempty"`
	ApplicationEndDate   *time.Time        `json:"application_end_date,omitempty"`
}

// ProgramOpenTermInput is one entry of the open-term set sent on a program update:
// an entry with ID updates that open term, one without creates a new open term.
type ProgramOpenTermInput struct {
	ID                   string     `json:"id,omitempty"`
	Name                 string     `json:"name"`
	StartDateTime        *time.Time `json:"start_date_time,omitempty"`
	EndDateTime          *time.Time `json:"end_date_time,omitempty"`
	ApplicationStartDate *time.Time `json:"application_start_date,omitempty"`
	ApplicationEndDate   *time.Time `json:"application_end_date,omitempty"`
}

// ProgramTermUpdateInput is the request body for updating a program term.
type ProgramTermUpdateInput struct {
	Name                 *string            `json:"name,omitempty"`
	Status               *ProgramTermStatus `json:"status,omitempty"`
	ActiveUsers          *int               `json:"active_users,omitempty"`
	StartDateTime        *time.Time         `json:"start_date_time,omitempty"`
	EndDateTime          *time.Time         `json:"end_date_time,omitempty"`
	ApplicationStartDate *time.Time         `json:"application_start_date,omitempty"`
	ApplicationEndDate   *time.Time         `json:"application_end_date,omitempty"`
}
