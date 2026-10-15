// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// ProgramManagementSummary provides the term visibility and tab counts used by
// the administrative program detail header.
type ProgramManagementSummary struct {
	HasOpenTerm   bool `json:"has_open_term"`
	HasClosedTerm bool `json:"has_closed_term"`
	Mentees       int  `json:"mentees"`
	PastMentees   int  `json:"past_mentees"`
	Applicants    int  `json:"applicants"`
	Mentors       int  `json:"mentors"`
	Terms         int  `json:"terms"`
}
