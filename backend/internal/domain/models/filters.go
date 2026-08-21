// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// PaginationMeta carries total count and current page parameters for list responses.
type PaginationMeta struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// UserFilter constrains list queries for users.
type UserFilter struct {
	Limit  int
	Offset int
	Search string // matches name, email, or lfid (ilike)
}

// UserProfileFilter constrains list queries for user profiles.
type UserProfileFilter struct {
	Limit       int
	Offset      int
	UserID      string
	ProfileType string // mentor | mentee
}

// ProgramFilter constrains list queries for programs.
type ProgramFilter struct {
	Limit  int
	Offset int
	Status string // pending | published | archived
	Search string // ilike on name
}

// ProgramTermFilter constrains list queries for program terms.
type ProgramTermFilter struct {
	Limit     int
	Offset    int
	ProgramID string
	Status    string // open | closed
}

// ProgramMemberFilter constrains list queries for program members.
type ProgramMemberFilter struct {
	Limit      int
	Offset     int
	MemberType string // program_admin | mentor
	Status     string
}

// ApplicationFilter constrains list queries for applications.
type ApplicationFilter struct {
	Limit  int
	Offset int
	UserID string
	Role   string // mentor | mentee
	Status string // pending | accepted | declined | withdrawn
}

// TaskFilter constrains list queries for tasks.
type TaskFilter struct {
	Limit      int
	Offset     int
	Status     string
	AssigneeID string
}
