// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// MentoredProgramStatus groups a mentor's program by its terms, as
// AdministeredProgramStatus does for a published program: open while it has an
// open term or no terms yet, completed once all its terms are closed.
type MentoredProgramStatus string

const (
	MentoredProgramStatusOpen      MentoredProgramStatus = "open"
	MentoredProgramStatusCompleted MentoredProgramStatus = "completed"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s MentoredProgramStatus) IsValid() bool {
	switch s {
	case MentoredProgramStatusOpen, MentoredProgramStatusCompleted:
		return true
	}
	return false
}

// MentoredProgramStats counts the program's rows across all its terms.
type MentoredProgramStats struct {
	// Mentees are the mentee applications whose status is accepted or graduated.
	Mentees int `json:"mentees"`
	// Applicants counts one mentee application per user per term.
	Applicants int `json:"applicants"`
	// TasksToReview counts submitted tasks of accepted mentees.
	TasksToReview int `json:"tasks_to_review"`
}

// MentoredProgram is one row of the caller's mentor programs list: a published
// program the caller is an active mentor of, with its status and counts.
type MentoredProgram struct {
	ID          string                `json:"id"`
	Slug        string                `json:"slug"`
	Name        string                `json:"name"`
	ProjectName *string               `json:"project_name,omitempty"`
	LogoURL     *string               `json:"logo_url,omitempty"`
	Status      MentoredProgramStatus `json:"status"`
	Stats       MentoredProgramStats  `json:"stats"`
}
