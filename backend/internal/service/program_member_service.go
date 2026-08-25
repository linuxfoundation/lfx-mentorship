// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var programMemberSvcTracer = otel.Tracer("program-members-service")

// ProgramMemberService orchestrates program member reads and writes.
type ProgramMemberService struct {
	repo         domain.ProgramMemberRepository
	programRepo  domain.ProgramRepository
	notifier     domain.Notifier
	inviteSecret string
}

// NewProgramMemberService returns a ProgramMemberService.
func NewProgramMemberService(repo domain.ProgramMemberRepository, programRepo domain.ProgramRepository, notifier domain.Notifier, inviteSecret string) *ProgramMemberService {
	return &ProgramMemberService{repo: repo, programRepo: programRepo, notifier: notifier, inviteSecret: inviteSecret}
}

// memberTransitions defines valid next statuses for each member status.
var memberTransitions = map[string]map[string]bool{
	"invited":   {"active": true, "declined": true, "pending": true},
	"requested": {"active": true, "declined": true, "pending": true},
	"active":    {"withdrawn": true, "pending": true},
	"pending":   {"active": true, "declined": true},
	"declined":  {},
	"withdrawn": {},
}

var validMemberTypes = map[string]bool{
	"program_admin": true,
	"mentor":        true,
}

// GetByID returns the program member with the given ID.
func (s *ProgramMemberService) GetByID(ctx context.Context, id string) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("member.id", id))

	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program member: %w", err)
	}
	return m, nil
}

// ListByProgram returns paginated members for a program.
func (s *ProgramMemberService) ListByProgram(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.ListByProgram")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID))

	members, meta, err := s.repo.ListByProgram(ctx, programID, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list program members: %w", err)
	}
	return members, meta, nil
}

// Create validates input and adds a member to a program.
// When member_type is "mentor", the member is created with status "invited" and
// a time-limited invite token is sent via the notifier.
func (s *ProgramMemberService) Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.Create")
	defer span.End()

	if input.UserID == "" {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	}
	if !validMemberTypes[input.MemberType] {
		return nil, fmt.Errorf("%w: member_type must be program_admin or mentor", domain.ErrInvalidInput)
	}

	// FR-018 / FR-023: members may only be added to a published program.
	prog, err := s.programRepo.GetByID(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program: %w", err)
	}
	if prog.Status != "published" {
		return nil, fmt.Errorf("%w: program must be published before adding members", domain.ErrInvalidInput)
	}

	// Mentors are placed in 'invited' status and notified; program_admins are 'active' immediately.
	if input.MemberType == "mentor" {
		if input.Status == nil {
			s := "invited"
			input.Status = &s
		}
	} else {
		if input.Status == nil {
			s := "active"
			input.Status = &s
		}
	}

	input.ID = uuid.New().String()
	m, err := s.repo.Create(ctx, programID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program member: %w", err)
	}

	// Send invite notification for mentors.
	if input.MemberType == "mentor" && s.inviteSecret != "" {
		token, tokenErr := auth.GenerateInviteToken(programID, input.UserID, s.inviteSecret)
		if tokenErr != nil {
			span.RecordError(tokenErr)
			// Non-fatal: log but don't fail the create.
		} else {
			s.notifier.NotifyMentorInvited(ctx, programID, input.UserID, token)
		}
	}

	return m, nil
}

// Update patches a program member, enforcing the status lifecycle.
func (s *ProgramMemberService) Update(ctx context.Context, id string, input models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("member.id", id))

	if input.Status != nil {
		current, err := s.repo.GetByID(ctx, id)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("get member: %w", err)
		}
		currentStatus := ""
		if current.Status != nil {
			currentStatus = *current.Status
		}
		if !memberTransitions[currentStatus][*input.Status] {
			return nil, fmt.Errorf("%w: cannot transition member from %q to %q", domain.ErrInvalidStateTransition, currentStatus, *input.Status)
		}
		if *input.Status == "declined" {
			s.notifier.NotifyMentorDeclined(ctx, current.ProgramID, current.UserID)
		}
	}

	m, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program member: %w", err)
	}
	return m, nil
}

// AcceptInvite validates a mentor invite token and transitions the member to active.
func (s *ProgramMemberService) AcceptInvite(ctx context.Context, token string) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.AcceptInvite")
	defer span.End()

	programID, userID, err := auth.ValidateInviteToken(token, s.inviteSecret)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidInput, err.Error())
	}

	span.SetAttributes(
		attribute.String("program.id", programID),
		attribute.String("user.id", userID),
	)

	// Find the invited member record.
	members, _, err := s.repo.ListByProgram(ctx, programID, models.ProgramMemberFilter{Limit: 100})
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("lookup member for invite: %w", err)
	}
	var memberID string
	for _, m := range members {
		if m.UserID == userID && m.Status != nil && *m.Status == "invited" {
			memberID = m.ID
			break
		}
	}
	if memberID == "" {
		return nil, fmt.Errorf("%w: no pending invite found for this user", domain.ErrInvalidInput)
	}

	activeStatus := "active"
	m, err := s.repo.Update(ctx, memberID, models.ProgramMemberUpdateInput{Status: &activeStatus})
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("accept invite: %w", err)
	}
	return m, nil
}

// DeclineInvite validates a mentor invite token and transitions the member to declined.
func (s *ProgramMemberService) DeclineInvite(ctx context.Context, token string) error {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.DeclineInvite")
	defer span.End()

	programID, userID, err := auth.ValidateInviteToken(token, s.inviteSecret)
	if err != nil {
		return fmt.Errorf("%w: %s", domain.ErrInvalidInput, err.Error())
	}

	span.SetAttributes(
		attribute.String("program.id", programID),
		attribute.String("user.id", userID),
	)

	members, _, err := s.repo.ListByProgram(ctx, programID, models.ProgramMemberFilter{Limit: 100})
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("lookup member for decline: %w", err)
	}
	var memberID string
	for _, m := range members {
		if m.UserID == userID && m.Status != nil && *m.Status == "invited" {
			memberID = m.ID
			break
		}
	}
	if memberID == "" {
		return fmt.Errorf("%w: no pending invite found for this user", domain.ErrInvalidInput)
	}

	declinedStatus := "declined"
	if _, err := s.repo.Update(ctx, memberID, models.ProgramMemberUpdateInput{Status: &declinedStatus}); err != nil {
		span.RecordError(err)
		return fmt.Errorf("decline invite: %w", err)
	}
	s.notifier.NotifyMentorDeclined(ctx, programID, userID)
	return nil
}

// Delete removes a program member.
func (s *ProgramMemberService) Delete(ctx context.Context, id string) error {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("member.id", id))

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program member: %w", err)
	}
	return nil
}
