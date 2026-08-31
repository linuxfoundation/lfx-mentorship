// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// MenteeStatus is the directory-facing application status shown on mentee
// cards, programs, and terms. Values stay aligned with applications.status
// for enrolled mentees (accepted, active, graduated).
type MenteeStatus string

const (
	MenteeStatusAccepted  MenteeStatus = "accepted"
	MenteeStatusActive    MenteeStatus = "active"
	MenteeStatusGraduated MenteeStatus = "graduated"
)

// IsValid reports whether the status is one of the directory-visible values.
func (s MenteeStatus) IsValid() bool {
	switch s {
	case MenteeStatusAccepted, MenteeStatusActive, MenteeStatusGraduated:
		return true
	}
	return false
}

// MenteeProgramRef is the featured (or listed) program on a mentee card.
type MenteeProgramRef struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Slug    string  `json:"slug"`
	LogoURL *string `json:"logo_url,omitempty"`
}

// MenteeProgramTerm is one term the mentee enrolled in on a program.
type MenteeProgramTerm struct {
	ID                string       `json:"id"`
	Name              string       `json:"name"`
	StartDateTime     *time.Time   `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time   `json:"end_date_time,omitempty"`
	ApplicationStatus MenteeStatus `json:"application_status"`
}

// MenteeProgram is a published program the mentee joined.
type MenteeProgram struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Slug        string                 `json:"slug"`
	Description *string                `json:"description,omitempty"`
	LogoURL     *string                `json:"logo_url,omitempty"`
	Status      MenteeStatus           `json:"status"`
	Skills      []string               `json:"skills"`
	Terms       []MenteeProgramTerm    `json:"terms"`
	Mentors     []ProgramCatalogMentor `json:"mentors"`
}

// MenteeItem is one public directory card: one mentee, and a featured program when they have one.
type MenteeItem struct {
	UserID       string                 `json:"user_id"`
	Name         *string                `json:"name,omitempty"`
	AvatarURL    *string                `json:"avatar_url,omitempty"`
	Introduction *string                `json:"introduction,omitempty"`
	Skills       []string               `json:"skills"`
	Status       MenteeStatus           `json:"status,omitempty"`
	JoinedAt     time.Time              `json:"joined_at"`
	Program      *MenteeProgramRef      `json:"program,omitempty"`
	Mentors      []ProgramCatalogMentor `json:"mentors"`
}

// MenteeDetail is the public mentee profile, including all published programs.
type MenteeDetail struct {
	MenteeItem
	GithubURL   *string         `json:"github_url,omitempty"`
	LinkedInURL *string         `json:"linkedin_url,omitempty"`
	Programs    []MenteeProgram `json:"programs"`
}

// MenteePage is a paginated, filterable directory response.
type MenteePage struct {
	Data []*MenteeItem  `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

// MenteeSummary is the unfiltered directory header: mentee and program totals.
type MenteeSummary struct {
	MenteeCount  int `json:"mentee_count"`
	ProgramCount int `json:"program_count"`
}
