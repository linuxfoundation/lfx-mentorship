// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// MentoredProgramTermStatus groups a mentor's program by the term shown on it:
// a started open term, an open term not started yet, or a closed term.
type MentoredProgramTermStatus string

const (
	MentoredProgramTermStatusActiveTerm MentoredProgramTermStatus = "active_term"
	MentoredProgramTermStatusUpcoming   MentoredProgramTermStatus = "upcoming"
	MentoredProgramTermStatusCompleted  MentoredProgramTermStatus = "completed"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s MentoredProgramTermStatus) IsValid() bool {
	switch s {
	case MentoredProgramTermStatusActiveTerm, MentoredProgramTermStatusUpcoming, MentoredProgramTermStatusCompleted:
		return true
	}
	return false
}

// MentoredProgramStats counts the chosen term's rows only.
type MentoredProgramStats struct {
	// Mentees are the applicants whose status is accepted or graduated.
	Mentees int `json:"mentees"`
	// Applicants counts one mentee application per user on the term.
	Applicants int `json:"applicants"`
	// TasksToReview counts submitted tasks of accepted mentees.
	TasksToReview int `json:"tasks_to_review"`
}

// MentoredProgram is one row of the caller's mentor programs list: a published
// program the caller is an active mentor of, with its chosen term and counts.
type MentoredProgram struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	ProjectName *string `json:"project_name,omitempty"`
	LogoURL     *string `json:"logo_url,omitempty"`
	// Term is the started open term, else the next open term, else the latest
	// closed term — the same choice as AdministeredProgram.Term. It is nil when
	// the program has no terms.
	Term       *ProgramTerm              `json:"term,omitempty"`
	TermStatus MentoredProgramTermStatus `json:"term_status"`
	Stats      MentoredProgramStats      `json:"stats"`
}
