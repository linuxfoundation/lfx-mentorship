// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"encoding/json"
	"time"
)

// UserProfile maps to the public.user_profiles table.
type UserProfile struct {
	ID                 string          `json:"id"`
	UserID             string          `json:"user_id"`
	ProfileType        string          `json:"profile_type"` // mentor | mentee
	Slug               *string         `json:"slug,omitempty"`
	FirstName          *string         `json:"first_name,omitempty"`
	LastName           *string         `json:"last_name,omitempty"`
	Email              *string         `json:"email,omitempty"`
	Phone              *string         `json:"phone,omitempty"`
	LogoURL            *string         `json:"logo_url,omitempty"`
	Introduction       *string         `json:"introduction,omitempty"`
	TermsAndConditions bool            `json:"terms_and_conditions"`
	NumberOfProjects   int             `json:"number_of_projects"`
	Address            json.RawMessage `json:"address,omitempty"`
	Demographics       json.RawMessage `json:"demographics,omitempty"`
	Socioeconomics     json.RawMessage `json:"socioeconomics,omitempty"`
	SkillSet           json.RawMessage `json:"skill_set,omitempty"`
	ProfileLinks       json.RawMessage `json:"profile_links,omitempty"`
	CreatedOn          time.Time       `json:"created_on"`
	UpdatedOn          time.Time       `json:"updated_on"`
}
