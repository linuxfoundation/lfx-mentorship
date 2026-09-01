// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// PlatformSummary is the aggregated marketing/landing summary counts.
// All counts are unfiltered by any caller-supplied criteria.
type PlatformSummary struct {
	// ProgramCount is the number of programs with status='published'.
	ProgramCount int `json:"program_count"`
	// AcceptingProgramCount is the subset of published programs that
	// currently have at least one open term whose application window
	// includes the current moment.
	AcceptingProgramCount int `json:"accepting_program_count"`
	// MentorCount is the number of distinct users who are an active
	// mentor member of at least one published program.
	MentorCount int `json:"mentor_count"`
	// GraduatedMenteeCount is the number of distinct users with at least
	// one mentee application in status='graduated' on a non-deleted term
	// of a published program.
	GraduatedMenteeCount int `json:"graduated_mentee_count"`
}
