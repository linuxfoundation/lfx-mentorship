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

// MaxOpenTermsPerProgram is the maximum number of concurrently open terms allowed (FR-003).
const MaxOpenTermsPerProgram = 4

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

// IsHistorical reports whether the term is closed and has ended; such terms
// can no longer be edited.
func (t *ProgramTerm) IsHistorical(now time.Time) bool {
	return t.Status == ProgramTermStatusClosed && t.EndDateTime != nil && now.After(*t.EndDateTime)
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

// ProgramTermReplaceInput is one entry of the full term set sent on a program
// update: an entry with ID updates that term, one without creates a new open term.
// Status is not accepted; term lifecycle changes use the close and reopen routes.
type ProgramTermReplaceInput struct {
	ID                   string     `json:"id,omitempty"`
	Name                 string     `json:"name"`
	StartDateTime        *time.Time `json:"start_date_time,omitempty"`
	EndDateTime          *time.Time `json:"end_date_time,omitempty"`
	ApplicationStartDate *time.Time `json:"application_start_date,omitempty"`
	ApplicationEndDate   *time.Time `json:"application_end_date,omitempty"`
}

// Matches reports whether the entry carries the term's current name and dates.
// Dates are compared to the millisecond because browser clients round-trip
// timestamps through JavaScript Date, which drops microseconds.
func (in ProgramTermReplaceInput) Matches(t *ProgramTerm) bool {
	sameTime := func(a, b *time.Time) bool {
		if a == nil || b == nil {
			return a == b
		}
		return a.Truncate(time.Millisecond).Equal(b.Truncate(time.Millisecond))
	}
	return in.Name == t.Name &&
		sameTime(in.StartDateTime, t.StartDateTime) &&
		sameTime(in.EndDateTime, t.EndDateTime) &&
		sameTime(in.ApplicationStartDate, t.ApplicationStartDate) &&
		sameTime(in.ApplicationEndDate, t.ApplicationEndDate)
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
