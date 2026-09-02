// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// MentorProgramTerm is one term on a program the mentor belongs to.
type MentorProgramTerm struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Status               string     `json:"status"` // open | closed
	StartDateTime        *time.Time `json:"start_date_time,omitempty"`
	EndDateTime          *time.Time `json:"end_date_time,omitempty"`
	ApplicationStartDate *time.Time `json:"application_start_date,omitempty"`
	ApplicationEndDate   *time.Time `json:"application_end_date,omitempty"`
}

// MentorProgram is a published program the mentor is an active member of.
type MentorProgram struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Slug        string                 `json:"slug"`
	Description *string                `json:"description,omitempty"`
	LogoURL     *string                `json:"logo_url,omitempty"`
	Skills      []string               `json:"skills"`
	Terms       []MentorProgramTerm    `json:"terms"`
	Mentors     []ProgramCatalogMentor `json:"mentors"`
}

// MentorItem is one public directory card: one mentor.
type MentorItem struct {
	UserID       string    `json:"user_id"`
	Name         *string   `json:"name,omitempty"`
	AvatarURL    *string   `json:"avatar_url,omitempty"`
	Introduction *string   `json:"introduction,omitempty"`
	Skills       []string  `json:"skills"`
	JoinedAt     time.Time `json:"joined_at"`
}

// MentorMentee is a mentee on a published program this mentor belongs to.
type MentorMentee struct {
	UserID       string       `json:"user_id"`
	Name         *string      `json:"name,omitempty"`
	AvatarURL    *string      `json:"avatar_url,omitempty"`
	Introduction *string      `json:"introduction,omitempty"`
	ProgramName  string       `json:"program_name"`
	TermName     string       `json:"term_name"`
	Status       MenteeStatus `json:"status"`
}

// MentorStats is the public profile sidebar counts.
type MentorStats struct {
	ProgramsMentoring int `json:"programs_mentoring"`
	CurrentMentees    int `json:"current_mentees"`
	MenteesGraduated  int `json:"mentees_graduated"`
}

// MentorDetail is the public mentor profile.
type MentorDetail struct {
	MentorItem
	GithubURL        *string         `json:"github_url,omitempty"`
	LinkedInURL      *string         `json:"linkedin_url,omitempty"`
	Programs         []MentorProgram `json:"programs"`
	CurrentMentees   []MentorMentee  `json:"current_mentees"`
	GraduatedMentees []MentorMentee  `json:"graduated_mentees"`
	Stats            MentorStats     `json:"stats"`
}

// MentorPage is a paginated, filterable directory response.
type MentorPage struct {
	Data []*MentorItem  `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

// MentorSummary is the unfiltered directory header: mentor and program totals.
type MentorSummary struct {
	MentorCount  int `json:"mentor_count"`
	ProgramCount int `json:"program_count"`
}
