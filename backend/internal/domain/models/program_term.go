// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// ProgramTerm maps to the public.program_terms table.
type ProgramTerm struct {
	ID                   string     `json:"id"`
	ProgramID            string     `json:"program_id"`
	Name                 string     `json:"name"`
	Status               string     `json:"status"` // open | closed
	ActiveUsers          int        `json:"active_users"`
	StartDateTime        *time.Time `json:"start_date_time,omitempty"`
	EndDateTime          *time.Time `json:"end_date_time,omitempty"`
	ApplicationStartDate *time.Time `json:"application_start_date,omitempty"`
	ApplicationEndDate   *time.Time `json:"application_end_date,omitempty"`
	CreatedOn            time.Time  `json:"created_on"`
	UpdatedOn            time.Time  `json:"updated_on"`
}

// ProgramTermCreateInput is the request body for creating a program term.
type ProgramTermCreateInput struct {
	ID                   string     `json:"id"`
	ProgramID            string     `json:"program_id"`
	Name                 string     `json:"name"`
	Status               string     `json:"status"`
	ActiveUsers          int        `json:"active_users"`
	StartDateTime        *time.Time `json:"start_date_time,omitempty"`
	EndDateTime          *time.Time `json:"end_date_time,omitempty"`
	ApplicationStartDate *time.Time `json:"application_start_date,omitempty"`
	ApplicationEndDate   *time.Time `json:"application_end_date,omitempty"`
}

// ProgramTermUpdateInput is the request body for updating a program term.
type ProgramTermUpdateInput struct {
	Name                 *string    `json:"name,omitempty"`
	Status               *string    `json:"status,omitempty"`
	ActiveUsers          *int       `json:"active_users,omitempty"`
	StartDateTime        *time.Time `json:"start_date_time,omitempty"`
	EndDateTime          *time.Time `json:"end_date_time,omitempty"`
	ApplicationStartDate *time.Time `json:"application_start_date,omitempty"`
	ApplicationEndDate   *time.Time `json:"application_end_date,omitempty"`
}
