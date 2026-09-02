// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

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
