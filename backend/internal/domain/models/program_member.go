// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// ProgramMember maps to the public.program_members table.
type ProgramMember struct {
	ID         string    `json:"id"`
	ProgramID  string    `json:"program_id"`
	UserID     string    `json:"user_id"`
	MemberType string    `json:"member_type"` // maintainer | mentor | apprentice
	Status     *string   `json:"status,omitempty"`
	Email      *string   `json:"email,omitempty"`
	CreatedOn  time.Time `json:"created_on"`
	UpdatedOn  time.Time `json:"updated_on"`
}

// ProgramMemberCreateInput is the request body for adding a program member.
type ProgramMemberCreateInput struct {
	ID         string  `json:"id"`
	UserID     string  `json:"user_id"`
	MemberType string  `json:"member_type"`
	Status     *string `json:"status,omitempty"`
	Email      *string `json:"email,omitempty"`
}

// ProgramMemberUpdateInput is the request body for updating a program member.
type ProgramMemberUpdateInput struct {
	Status *string `json:"status,omitempty"`
	Email  *string `json:"email,omitempty"`
}

// ProgramAdmin maps to the public.program_admins table.
type ProgramAdmin struct {
	ID            string    `json:"id"`
	ProgramID     string    `json:"program_id"`
	UserProfileID string    `json:"user_profile_id"`
	Role          string    `json:"role"`
	CreatedOn     time.Time `json:"created_on"`
	UpdatedOn     time.Time `json:"updated_on"`
}

// ProgramAdminCreateInput is the request body for adding a program admin.
type ProgramAdminCreateInput struct {
	UserProfileID string `json:"user_profile_id"`
	Role          string `json:"role"`
}
