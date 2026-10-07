// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type DatabaseBuilder struct {
	programs     domain.ProgramRepository
	members      domain.ProgramMemberRepository
	users        domain.UserRepository
	approvers    domain.ApproverRepository
	terms        domain.ProgramTermRepository
	applications domain.ApplicationRepository
	tasks        domain.TaskRepository
}

func NewDatabaseBuilder(programs domain.ProgramRepository, members domain.ProgramMemberRepository, users domain.UserRepository, terms domain.ProgramTermRepository, applications domain.ApplicationRepository, tasks domain.TaskRepository, approvers domain.ApproverRepository) *DatabaseBuilder {
	return &DatabaseBuilder{programs: programs, members: members, users: users, approvers: approvers, terms: terms, applications: applications, tasks: tasks}
}

func (b *DatabaseBuilder) Build(ctx context.Context, marker domain.FGAOutboxMarker) (Message, error) {
	if marker.DesiredOperation == "delete_access" {
		return DeleteAccess(marker.ObjectType, marker.ObjectUID)
	}
	if marker.MarkerKind == "membership" {
		if marker.Relation == nil || marker.Username == nil {
			return Message{}, fmt.Errorf("membership marker %d is missing relation or username", marker.ID)
		}
		if marker.ObjectType == "project" {
			return Message{}, fmt.Errorf("project membership markers are owned by project-service")
		}
		if marker.DesiredOperation == "remove" {
			return MemberRemove(marker.ObjectType, marker.ObjectUID, *marker.Username, *marker.Relation)
		}
		if marker.ObjectType == "mentorship_approver_team" {
			return b.buildApproverMembership(ctx, marker)
		}
		return b.buildMembership(ctx, marker)
	}
	switch marker.ObjectType {
	case "mentorship_program":
		return b.buildProgram(ctx, marker.ObjectUID)
	case "mentorship_application":
		return b.buildApplication(ctx, marker.ObjectUID)
	case "mentorship_task":
		return b.buildTask(ctx, marker.ObjectUID)
	default:
		return Message{}, fmt.Errorf("unsupported FGA object type %q", marker.ObjectType)
	}
}

func (b *DatabaseBuilder) buildApproverMembership(ctx context.Context, marker domain.FGAOutboxMarker) (Message, error) {
	lfids, err := b.approvers.ListLFIDs(ctx)
	if err != nil {
		return Message{}, err
	}
	for _, lfid := range lfids {
		if lfid == *marker.Username {
			return MemberPut("mentorship_approver_team", "global", *marker.Username, []string{*marker.Relation})
		}
	}
	return MemberRemove("mentorship_approver_team", "global", *marker.Username, *marker.Relation)
}

func (b *DatabaseBuilder) buildProgram(ctx context.Context, id string) (Message, error) {
	program, err := b.programs.GetByID(ctx, id)
	if err != nil {
		return Message{}, err
	}
	members, err := b.listAllMembers(ctx, id)
	if err != nil {
		return Message{}, err
	}
	writers, mentors := []string{}, []string{}
	for _, member := range members {
		if member.Status == nil || *member.Status != models.ProgramMemberStatusActive {
			continue
		}
		lfid, err := b.requireLFID(ctx, member.UserID)
		if err != nil {
			return Message{}, fmt.Errorf("resolve program member: %w", err)
		}
		if member.MemberType == models.MemberTypeProgramAdmin {
			writers = append(writers, lfid)
		}
		if member.MemberType == models.MemberTypeMentor {
			mentors = append(mentors, lfid)
		}
	}
	return ProgramAccess(program, writers, mentors)
}

func (b *DatabaseBuilder) buildApplication(ctx context.Context, id string) (Message, error) {
	application, err := b.applications.GetByID(ctx, id)
	if err != nil {
		return Message{}, err
	}
	term, err := b.terms.GetByID(ctx, application.ProgramTermID)
	if err != nil {
		return Message{}, err
	}
	lfid, err := b.requireLFID(ctx, application.UserID)
	if err != nil {
		return Message{}, fmt.Errorf("resolve application user: %w", err)
	}
	return ApplicationAccess(application, term.ProgramID, lfid)
}

func (b *DatabaseBuilder) buildTask(ctx context.Context, id string) (Message, error) {
	task, err := b.tasks.GetByID(ctx, id)
	if err != nil {
		return Message{}, err
	}
	if task.ApplicationID == nil || *task.ApplicationID == "" {
		return DeleteAccess(taskObject, task.ID)
	}
	lfid, err := b.requireLFID(ctx, task.AssigneeID)
	if err != nil {
		return Message{}, fmt.Errorf("resolve task assignee: %w", err)
	}
	return TaskAccess(task, *task.ApplicationID, lfid)
}

// requireLFID returns the user's LFID; a user without one cannot be represented in a tuple.
func (b *DatabaseBuilder) requireLFID(ctx context.Context, userID string) (string, error) {
	user, err := b.users.GetByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get user %s: %w", userID, err)
	}
	if user.LFID == nil || *user.LFID == "" {
		return "", fmt.Errorf("user %s has no LFID", userID)
	}
	return *user.LFID, nil
}

func (b *DatabaseBuilder) buildMembership(ctx context.Context, marker domain.FGAOutboxMarker) (Message, error) {
	members, err := b.listAllMembers(ctx, marker.ObjectUID)
	if err != nil {
		return Message{}, err
	}
	for _, member := range members {
		if !isActiveMembership(member, *marker.Relation) {
			continue
		}
		lfid, err := b.requireLFID(ctx, member.UserID)
		if err != nil {
			return Message{}, fmt.Errorf("resolve membership user: %w", err)
		}
		if lfid == *marker.Username {
			return MemberPut("mentorship_program", marker.ObjectUID, *marker.Username, []string{*marker.Relation})
		}
	}
	return MemberRemove("mentorship_program", marker.ObjectUID, *marker.Username, *marker.Relation)
}

func (b *DatabaseBuilder) listAllMembers(ctx context.Context, programID string) ([]*models.ProgramMember, error) {
	const pageSize = 100
	var all []*models.ProgramMember
	for offset := 0; ; offset += pageSize {
		members, meta, err := b.members.ListByProgram(ctx, programID, models.ProgramMemberFilter{Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		all = append(all, members...)
		if len(members) == 0 || meta == nil || offset+len(members) >= meta.Total {
			return all, nil
		}
	}
}

func isActiveMembership(member *models.ProgramMember, relation string) bool {
	if member.Status == nil || *member.Status != models.ProgramMemberStatusActive {
		return false
	}
	return (relation == "writer" && member.MemberType == models.MemberTypeProgramAdmin) || (relation == "mentor" && member.MemberType == models.MemberTypeMentor)
}
