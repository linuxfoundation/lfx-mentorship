// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// ProgramMemberStatus enumerates valid values for program_members.status.
// A turned-down mentor invite is "declined", matching ApplicationStatusDeclined;
// "rejected" is reserved for ProgramStatus (program moderation).
type ProgramMemberStatus string

const (
	ProgramMemberStatusInvited   ProgramMemberStatus = "invited"
	ProgramMemberStatusRequested ProgramMemberStatus = "requested"
	ProgramMemberStatusPending   ProgramMemberStatus = "pending"
	ProgramMemberStatusActive    ProgramMemberStatus = "active"
	ProgramMemberStatusDeclined  ProgramMemberStatus = "declined"
	ProgramMemberStatusWithdrawn ProgramMemberStatus = "withdrawn"
)

// IsValid reports whether the status value is one of the allowed enum members.
func (s ProgramMemberStatus) IsValid() bool {
	switch s {
	case ProgramMemberStatusInvited, ProgramMemberStatusRequested, ProgramMemberStatusPending,
		ProgramMemberStatusActive, ProgramMemberStatusDeclined, ProgramMemberStatusWithdrawn:
		return true
	}
	return false
}

// MemberType enumerates valid values for program_members.member_type.
type MemberType string

const (
	MemberTypeProgramAdmin MemberType = "program_admin"
	MemberTypeMentor       MemberType = "mentor"
)

// IsValid reports whether the member type is one of the allowed enum members.
func (t MemberType) IsValid() bool {
	switch t {
	case MemberTypeProgramAdmin, MemberTypeMentor:
		return true
	}
	return false
}

// ProgramMember maps to the public.program_members table.
type ProgramMember struct {
	ID         string               `json:"id"`
	ProgramID  string               `json:"program_id"`
	UserID     string               `json:"user_id"`
	MemberType MemberType           `json:"member_type"`
	Status     *ProgramMemberStatus `json:"status,omitempty"`
	Email      *string              `json:"email,omitempty"`
	CreatedOn  time.Time            `json:"created_on"`
	UpdatedOn  time.Time            `json:"updated_on"`
}

// ProgramMemberCreateInput is the request body for adding a program member.
type ProgramMemberCreateInput struct {
	ID         string               `json:"id"`
	UserID     string               `json:"user_id"`
	MemberType MemberType           `json:"member_type"`
	Status     *ProgramMemberStatus `json:"status,omitempty"`
	Email      *string              `json:"email,omitempty"`
}

// ProgramMemberUpdateInput is the request body for updating a program member.
type ProgramMemberUpdateInput struct {
	Status *ProgramMemberStatus `json:"status,omitempty"`
	Email  *string              `json:"email,omitempty"`
}
