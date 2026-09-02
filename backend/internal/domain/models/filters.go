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

// MenteeFilter constrains public mentee directory queries.
type MenteeFilter struct {
	Limit  int
	Offset int
	Search string // case-insensitive match on mentee name
	Skill  string // case-insensitive exact match on a mentee skill
	Status string // active | graduated; omit or all for every accepted mentee
}

// MentorFilter constrains public mentor directory queries.
type MentorFilter struct {
	Limit  int
	Offset int
	Search string // case-insensitive match on mentor name
	Skill  string // case-insensitive exact match on a mentor skill
}

// ProgramFilter constrains list queries for programs.
type ProgramFilter struct {
	Limit           int
	Offset          int
	Status          string // programs.status: draft | submitted | published | hidden | rejected | archived
	Search          string // ilike on name
	Skill           string // catalog only: case-insensitive exact match on a program skill
	DiscoveryStatus string // catalog only: acceptance | in-progress | completed
	SortBy          string // catalog only: accepting_first | completed_first | name_asc | name_desc | updated_oldest | updated_newest
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
	Limit          int
	Offset         int
	UserID         string
	Role           string // mentor | mentee
	Status         string // pending | accepted | declined | withdrawn
	TasksSubmitted *bool  // nil = no filter
}

// TaskFilter constrains list queries for tasks.
type TaskFilter struct {
	Limit      int
	Offset     int
	Status     string
	AssigneeID string
}
