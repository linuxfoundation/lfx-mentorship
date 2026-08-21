// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"encoding/json"
	"time"
)

// ProgramStatus enumerates valid values for programs.status.
type ProgramStatus string

const (
	ProgramStatusDraft     ProgramStatus = "draft"
	ProgramStatusSubmitted ProgramStatus = "submitted"
	ProgramStatusPublished ProgramStatus = "published"
	ProgramStatusRejected  ProgramStatus = "rejected"
	ProgramStatusArchived  ProgramStatus = "archived"
	ProgramStatusHidden    ProgramStatus = "hidden"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s ProgramStatus) IsValid() bool {
	switch s {
	case ProgramStatusDraft, ProgramStatusSubmitted, ProgramStatusPublished,
		ProgramStatusRejected, ProgramStatusArchived, ProgramStatusHidden:
		return true
	}
	return false
}

// Program maps to the public.programs table.
type Program struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Slug               string          `json:"slug"`
	Status             ProgramStatus   `json:"status"`
	IsPaid             bool            `json:"is_paid"`
	Description        *string         `json:"description,omitempty"`
	LogoURL            *string         `json:"logo_url,omitempty"`
	WebsiteURL         *string         `json:"website_url,omitempty"`
	RepoLink           *string         `json:"repo_link,omitempty"`
	CodeOfConduct      *string         `json:"code_of_conduct,omitempty"`
	Industry           *string         `json:"industry,omitempty"`
	Color              *string         `json:"color,omitempty"`
	LFID               *string         `json:"lfid,omitempty"`
	CIIProjectID       *string         `json:"cii_project_id,omitempty"`
	AcceptApplications bool            `json:"accept_applications"`
	TermsAndConditions bool            `json:"terms_and_conditions"`
	ProgramTermStatus  *string         `json:"program_term_status,omitempty"`
	DiscoverSortRank   int             `json:"discover_sort_rank"`
	AmountRaised       float64         `json:"amount_raised"`
	MenteeNeeds        json.RawMessage `json:"mentee_needs,omitempty"`
	TaskTemplates      json.RawMessage `json:"task_templates,omitempty"`
	CreatedOn          time.Time       `json:"created_on"`
	UpdatedOn          time.Time       `json:"updated_on"`
}

// ProgramCreateInput is the request body for creating a program.
type ProgramCreateInput struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Slug               string          `json:"slug"`
	Status             ProgramStatus   `json:"status"`
	IsPaid             bool            `json:"is_paid"`
	Description        *string         `json:"description,omitempty"`
	LogoURL            *string         `json:"logo_url,omitempty"`
	WebsiteURL         *string         `json:"website_url,omitempty"`
	RepoLink           *string         `json:"repo_link,omitempty"`
	CodeOfConduct      *string         `json:"code_of_conduct,omitempty"`
	Industry           *string         `json:"industry,omitempty"`
	Color              *string         `json:"color,omitempty"`
	LFID               *string         `json:"lfid,omitempty"`
	CIIProjectID       *string         `json:"cii_project_id,omitempty"`
	AcceptApplications bool            `json:"accept_applications"`
	TermsAndConditions bool            `json:"terms_and_conditions"`
	MenteeNeeds        json.RawMessage `json:"mentee_needs,omitempty"`
	TaskTemplates      json.RawMessage `json:"task_templates,omitempty"`
}

// ProgramUpdateInput is the request body for updating a program.
type ProgramUpdateInput struct {
	Name               *string         `json:"name,omitempty"`
	Slug               *string         `json:"slug,omitempty"`
	Status             *ProgramStatus  `json:"status,omitempty"`
	IsPaid             *bool           `json:"is_paid,omitempty"`
	Description        *string         `json:"description,omitempty"`
	LogoURL            *string         `json:"logo_url,omitempty"`
	WebsiteURL         *string         `json:"website_url,omitempty"`
	RepoLink           *string         `json:"repo_link,omitempty"`
	CodeOfConduct      *string         `json:"code_of_conduct,omitempty"`
	Industry           *string         `json:"industry,omitempty"`
	Color              *string         `json:"color,omitempty"`
	LFID               *string         `json:"lfid,omitempty"`
	CIIProjectID       *string         `json:"cii_project_id,omitempty"`
	AcceptApplications *bool           `json:"accept_applications,omitempty"`
	TermsAndConditions *bool           `json:"terms_and_conditions,omitempty"`
	ProgramTermStatus  *string         `json:"program_term_status,omitempty"`
	DiscoverSortRank   *int            `json:"discover_sort_rank,omitempty"`
	MenteeNeeds        json.RawMessage `json:"mentee_needs,omitempty"`
	TaskTemplates      json.RawMessage `json:"task_templates,omitempty"`
}

// ProgramSkill maps to the public.program_skills table.
type ProgramSkill struct {
	ID        string    `json:"id"`
	ProgramID string    `json:"program_id"`
	Skill     string    `json:"skill"`
	CreatedOn time.Time `json:"created_on"`
	UpdatedOn time.Time `json:"updated_on"`
}

// ProgramSkillCreateInput is the request body for adding a program skill.
type ProgramSkillCreateInput struct {
	Skill string `json:"skill"`
}

// ProgramFundingStats maps to the public.program_funding_stats table.
type ProgramFundingStats struct {
	ID           string    `json:"id"`
	ProgramID    string    `json:"program_id"`
	AmountRaised float64   `json:"amount_raised"`
	CreatedOn    time.Time `json:"created_on"`
	UpdatedOn    time.Time `json:"updated_on"`
}
