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

// ApplicationWindowState is where a moment falls relative to a term's application window.
type ApplicationWindowState string

const (
	// ApplicationWindowUnset means the term lacks a start or end date, so it takes no applications.
	ApplicationWindowUnset    ApplicationWindowState = "unset"
	ApplicationWindowUpcoming ApplicationWindowState = "upcoming"
	ApplicationWindowOpen     ApplicationWindowState = "open"
	ApplicationWindowClosed   ApplicationWindowState = "closed"
)

// ApplicationWindow places now against the term's application window. As in legacy, the window is
// compared by UTC calendar day and both days are inclusive: it opens at 00:00 UTC on the start date
// and closes after 23:59:59 UTC on the end date, whatever time of day the dates were stored with.
func (t *ProgramTerm) ApplicationWindow(now time.Time) ApplicationWindowState {
	if t.ApplicationStartDate == nil || t.ApplicationEndDate == nil {
		return ApplicationWindowUnset
	}
	today := utcDay(now)
	if today.Before(utcDay(*t.ApplicationStartDate)) {
		return ApplicationWindowUpcoming
	}
	if today.After(utcDay(*t.ApplicationEndDate)) {
		return ApplicationWindowClosed
	}
	return ApplicationWindowOpen
}

// utcDay truncates t to midnight UTC of its UTC calendar day.
func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// DiscoveryLabel returns the public-facing term state label per FR-017.
// Mapping: open+window future→"Coming Soon"; open+in window→"Apply Now";
// open+window past or unset→"In Progress"; closed/deleted→"Completed".
func (t *ProgramTerm) DiscoveryLabel(now time.Time) string {
	if t.Status != ProgramTermStatusOpen {
		return "Completed"
	}
	switch t.ApplicationWindow(now) {
	case ApplicationWindowUpcoming:
		return "Coming Soon"
	case ApplicationWindowOpen:
		return "Apply Now"
	default:
		return "In Progress"
	}
}

// AcceptsApplications reports whether the term takes applications at now: it is
// open and now is inside its application window. ApplicationService.Create
// enforces the same rule on submit.
func (t *ProgramTerm) AcceptsApplications(now time.Time) bool {
	return t.Status == ProgramTermStatusOpen && t.ApplicationWindow(now) == ApplicationWindowOpen
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
