// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

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
	users        domain.UserRepository
	notifier     domain.Notifier
	inviteSecret string
}

// NewProgramMemberService returns a ProgramMemberService.
func NewProgramMemberService(repo domain.ProgramMemberRepository, programRepo domain.ProgramRepository, users domain.UserRepository, notifier domain.Notifier, inviteSecret string) *ProgramMemberService {
	return &ProgramMemberService{repo: repo, programRepo: programRepo, users: users, notifier: notifier, inviteSecret: inviteSecret}
}

func (s *ProgramMemberService) assertActiveProgramAdmin(ctx context.Context, programID, actorID string) error {
	if actorID == "" {
		return fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}
	if auth.IsGatewayPrincipal(ctx) {
		return nil
	}
	_, err := s.repo.FindActiveProgramAdminByProgramAndUser(ctx, programID, actorID)
	if err != nil {
		if !errors.Is(err, domain.ErrProgramMemberNotFound) {
			return fmt.Errorf("find actor membership: %w", err)
		}
		return fmt.Errorf("%w: actor must be an active program_admin", domain.ErrForbidden)
	}
	return nil
}

// memberTransitions defines the statuses a program admin may move a member
// to. A mentor's request is the mentor's to make or withdraw: an admin
// approves, declines, or deletes it, so requested/pending → withdrawn and
// withdrawn → requested are absent here and reachable only through
// WithdrawMine and RequestMentorship.
var memberTransitions = map[models.ProgramMemberStatus]map[models.ProgramMemberStatus]bool{
	models.ProgramMemberStatusInvited: {
		models.ProgramMemberStatusActive:   true,
		models.ProgramMemberStatusDeclined: true,
		models.ProgramMemberStatusPending:  true,
	},
	models.ProgramMemberStatusRequested: {
		models.ProgramMemberStatusActive:   true,
		models.ProgramMemberStatusDeclined: true,
		models.ProgramMemberStatusPending:  true,
	},
	models.ProgramMemberStatusActive: {
		models.ProgramMemberStatusWithdrawn: true,
		models.ProgramMemberStatusPending:   true,
	},
	models.ProgramMemberStatusPending: {
		models.ProgramMemberStatusActive:   true,
		models.ProgramMemberStatusDeclined: true,
	},
	models.ProgramMemberStatusDeclined:  {},
	models.ProgramMemberStatusWithdrawn: {},
}

// selfWithdrawableStatuses are the statuses a mentor may withdraw their own
// row from. An active mentor is removed by a program admin, not by themselves.
var selfWithdrawableStatuses = []models.ProgramMemberStatus{
	models.ProgramMemberStatusRequested,
	models.ProgramMemberStatusPending,
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

func (s *ProgramMemberService) ListMentorManagement(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMentorManagementRow, *models.PaginationMeta, error) {
	return s.repo.ListMentorManagement(ctx, programID, filter)
}

// Create validates input and adds a member to a program.
// When member_type is "mentor" and no status is given, the member is created
// with status "invited"; only an invited mentor is sent a time-limited invite
// token via the notifier.
func (s *ProgramMemberService) Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.Create")
	defer span.End()

	input.LFID = strings.TrimSpace(input.LFID)
	if input.UserID == "" && input.LFID == "" {
		return nil, fmt.Errorf("%w: user_id or lfid is required", domain.ErrInvalidInput)
	}
	if input.UserID != "" && input.LFID != "" {
		return nil, fmt.Errorf("%w: set user_id or lfid, not both", domain.ErrInvalidInput)
	}
	if !input.MemberType.IsValid() {
		return nil, fmt.Errorf("%w: member_type must be program_admin or mentor", domain.ErrInvalidInput)
	}

	// FR-018 / FR-023: members may only be added to a published program.
	prog, err := s.programRepo.GetByID(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program: %w", err)
	}
	if prog.Status != models.ProgramStatusPublished {
		return nil, fmt.Errorf("%w: program must be published before adding members", domain.ErrInvalidInput)
	}

	if input.LFID != "" {
		user, err := s.users.GetByLFID(ctx, input.LFID)
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, fmt.Errorf("%w: lfid %q has no Mentorship account; they must sign in once before they can be invited", domain.ErrIneligible, input.LFID)
		}
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("resolve lfid: %w", err)
		}
		input.UserID = user.ID
	}

	// Mentors are placed in 'invited' status and notified; program_admins are 'active' immediately.
	if input.Status == nil {
		defaultStatus := models.ProgramMemberStatusActive
		if input.MemberType == models.MemberTypeMentor {
			defaultStatus = models.ProgramMemberStatusInvited
		}
		input.Status = &defaultStatus
	} else if !input.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid member status %q", domain.ErrInvalidInput, *input.Status)
	}

	input.ID = uuid.New().String()
	m, err := s.repo.Create(ctx, programID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program member: %w", err)
	}

	// Only an invited mentor gets an invite email; a mentor created as
	// requested asked to join and must not be invited back.
	if input.MemberType == models.MemberTypeMentor && *input.Status == models.ProgramMemberStatusInvited && s.inviteSecret != "" {
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

// ListMine returns the caller's own memberships in any status.
func (s *ProgramMemberService) ListMine(ctx context.Context, userID string, filter models.ProgramMemberFilter) ([]*models.ProgramMembership, *models.PaginationMeta, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.ListMine")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))

	if userID == "" {
		return nil, nil, fmt.Errorf("%w: caller identity is required", domain.ErrUnauthorized)
	}
	if filter.MemberType != "" && !models.MemberType(filter.MemberType).IsValid() {
		return nil, nil, fmt.Errorf("%w: member_type must be program_admin or mentor", domain.ErrInvalidInput)
	}

	memberships, meta, err := s.repo.ListByUser(ctx, userID, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list my program memberships: %w", err)
	}
	return memberships, meta, nil
}

// RequestMentorship records the caller's own request to mentor a published
// program as a requested mentor row. A withdrawn row is reopened in place, as
// the table allows one row per program, user and member type. Every other
// existing row is a conflict: a declined request cannot be re-requested,
// matching the applications rule (FR-030).
//
// No invite email is sent: the caller asked to join and needs no invitation.
func (s *ProgramMemberService) RequestMentorship(ctx context.Context, programID, userID string) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.RequestMentorship")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID), attribute.String("user.id", userID))

	if userID == "" {
		return nil, fmt.Errorf("%w: caller identity is required", domain.ErrUnauthorized)
	}
	if _, err := uuid.Parse(programID); err != nil {
		return nil, fmt.Errorf("%w: program_id must be a UUID", domain.ErrInvalidInput)
	}

	prog, err := s.programRepo.GetByID(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program: %w", err)
	}
	// This route has no FGA program relation, so only a program visible to any
	// signed-in user may be acknowledged: published, or pending as a 400. Every
	// other status is hidden from non-owners (FR-009) and must stay a 404.
	switch prog.Status {
	case models.ProgramStatusPublished:
	case models.ProgramStatusPending:
		return nil, fmt.Errorf("%w: program must be published before requesting to mentor", domain.ErrInvalidInput)
	default:
		return nil, domain.ErrProgramNotFound
	}

	requested := models.ProgramMemberStatusRequested
	existing, err := s.repo.FindByProgramUserAndType(ctx, programID, userID, models.MemberTypeMentor)
	switch {
	case errors.Is(err, domain.ErrProgramMemberNotFound):
		m, err := s.repo.Create(ctx, programID, models.ProgramMemberCreateInput{
			ID:         uuid.New().String(),
			UserID:     userID,
			MemberType: models.MemberTypeMentor,
			Status:     &requested,
		})
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("create mentor request: %w", err)
		}
		return m, nil
	case err != nil:
		span.RecordError(err)
		return nil, fmt.Errorf("find existing mentor membership: %w", err)
	}

	if existing.Status == nil || *existing.Status != models.ProgramMemberStatusWithdrawn {
		var current models.ProgramMemberStatus
		if existing.Status != nil {
			current = *existing.Status
		}
		return nil, fmt.Errorf("%w: a mentor membership for this program is already %q", domain.ErrConflict, current)
	}

	m, err := s.repo.UpdateIfStatus(ctx, existing.ID, []models.ProgramMemberStatus{models.ProgramMemberStatusWithdrawn}, models.ProgramMemberUpdateInput{Status: &requested})
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("reopen mentor request: %w", err)
	}
	return m, nil
}

// WithdrawMine moves the caller's own requested or pending mentor row to
// withdrawn. A row that is not the caller's mentor row is reported as not
// found, so member IDs cannot be probed.
func (s *ProgramMemberService) WithdrawMine(ctx context.Context, id, userID string) error {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.WithdrawMine")
	defer span.End()
	span.SetAttributes(attribute.String("member.id", id), attribute.String("user.id", userID))

	if userID == "" {
		return fmt.Errorf("%w: caller identity is required", domain.ErrUnauthorized)
	}
	if _, err := uuid.Parse(id); err != nil {
		return domain.ErrProgramMemberNotFound
	}

	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("get member for withdraw: %w", err)
	}
	if current.UserID != userID || current.MemberType != models.MemberTypeMentor {
		return domain.ErrProgramMemberNotFound
	}
	if current.Status == nil || !slices.Contains(selfWithdrawableStatuses, *current.Status) {
		var status models.ProgramMemberStatus
		if current.Status != nil {
			status = *current.Status
		}
		return fmt.Errorf("%w: cannot withdraw a mentor membership that is %q", domain.ErrInvalidStateTransition, status)
	}

	withdrawn := models.ProgramMemberStatusWithdrawn
	if _, err := s.repo.UpdateIfStatus(ctx, id, selfWithdrawableStatuses, models.ProgramMemberUpdateInput{Status: &withdrawn}); err != nil {
		span.RecordError(err)
		return fmt.Errorf("withdraw mentor membership: %w", err)
	}
	return nil
}

// Update patches a program member, enforcing program ownership, admin authorization,
// and the status lifecycle.
func (s *ProgramMemberService) Update(ctx context.Context, programID, id string, input models.ProgramMemberUpdateInput, actorID string) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID), attribute.String("member.id", id), attribute.String("actor.id", actorID))

	if input.Status != nil {
		// Validate before any authorization or row lookups so bad enum values
		// consistently surface as invalid input.
		if !input.Status.IsValid() {
			return nil, fmt.Errorf("%w: invalid member status %q", domain.ErrInvalidInput, *input.Status)
		}
	}

	if err := s.assertActiveProgramAdmin(ctx, programID, actorID); err != nil {
		return nil, err
	}

	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get member: %w", err)
	}
	if current.ProgramID != programID {
		return nil, domain.ErrProgramMemberNotFound
	}

	if input.Status == nil {
		m, err := s.repo.Update(ctx, id, input)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("update program member: %w", err)
		}
		return m, nil
	}

	var currentStatus models.ProgramMemberStatus
	if current.Status != nil {
		currentStatus = *current.Status
	}
	if !memberTransitions[currentStatus][*input.Status] {
		return nil, fmt.Errorf("%w: cannot transition member from %q to %q", domain.ErrInvalidStateTransition, currentStatus, *input.Status)
	}
	// The transition was validated against currentStatus, so the write must
	// apply only while the row is still in it.
	m, err := s.repo.UpdateIfStatus(ctx, id, []models.ProgramMemberStatus{currentStatus}, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program member: %w", err)
	}
	// Revoking an invite (invited → declined) is not a reply to anything the mentor asked for.
	if currentStatus == models.ProgramMemberStatusRequested && *input.Status == models.ProgramMemberStatusDeclined {
		s.notifier.NotifyMentorDeclined(ctx, current.ProgramID, current.UserID)
	}
	return m, nil
}

// AcceptInvite validates a mentor invite token and transitions the member to active.
func (s *ProgramMemberService) AcceptInvite(ctx context.Context, token, actorID string) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.AcceptInvite")
	defer span.End()

	programID, userID, err := auth.ValidateInviteToken(token, s.inviteSecret)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidInput, err.Error())
	}
	if actorID == "" || actorID != userID {
		return nil, fmt.Errorf("%w: invite belongs to a different user", domain.ErrForbidden)
	}

	span.SetAttributes(
		attribute.String("program.id", programID),
		attribute.String("user.id", userID),
	)

	// Find the invited member record.
	memberID, err := s.findInvitedMentor(ctx, programID, userID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	activeStatus := models.ProgramMemberStatusActive
	m, err := s.repo.UpdateIfStatus(ctx, memberID, []models.ProgramMemberStatus{models.ProgramMemberStatusInvited}, models.ProgramMemberUpdateInput{Status: &activeStatus})
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("accept invite: %w", err)
	}
	s.notifier.NotifyAdminMentorAccepted(ctx, programID, userID)
	return m, nil
}

// DeclineInvite validates a mentor invite token and transitions the member to declined.
func (s *ProgramMemberService) DeclineInvite(ctx context.Context, token, actorID string) error {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.DeclineInvite")
	defer span.End()

	programID, userID, err := auth.ValidateInviteToken(token, s.inviteSecret)
	if err != nil {
		return fmt.Errorf("%w: %s", domain.ErrInvalidInput, err.Error())
	}
	if actorID == "" || actorID != userID {
		return fmt.Errorf("%w: invite belongs to a different user", domain.ErrForbidden)
	}

	span.SetAttributes(
		attribute.String("program.id", programID),
		attribute.String("user.id", userID),
	)

	memberID, err := s.findInvitedMentor(ctx, programID, userID)
	if err != nil {
		span.RecordError(err)
		return err
	}

	declinedStatus := models.ProgramMemberStatusDeclined
	if _, err := s.repo.UpdateIfStatus(ctx, memberID, []models.ProgramMemberStatus{models.ProgramMemberStatusInvited}, models.ProgramMemberUpdateInput{Status: &declinedStatus}); err != nil {
		span.RecordError(err)
		return fmt.Errorf("decline invite: %w", err)
	}
	s.notifier.NotifyAdminMentorDeclined(ctx, programID, userID)
	return nil
}

// findInvitedMentor returns the ID of the user's invited mentor row on the program.
func (s *ProgramMemberService) findInvitedMentor(ctx context.Context, programID, userID string) (string, error) {
	m, err := s.repo.FindByProgramUserAndType(ctx, programID, userID, models.MemberTypeMentor)
	if err != nil && !errors.Is(err, domain.ErrProgramMemberNotFound) {
		return "", fmt.Errorf("lookup invited mentor: %w", err)
	}
	if m == nil || m.Status == nil || *m.Status != models.ProgramMemberStatusInvited {
		return "", fmt.Errorf("%w: no pending invite found for this user", domain.ErrInvalidInput)
	}
	return m.ID, nil
}

// Delete removes a program member.
func (s *ProgramMemberService) Delete(ctx context.Context, programID, id, actorID string) error {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID), attribute.String("member.id", id), attribute.String("actor.id", actorID))

	if err := s.assertActiveProgramAdmin(ctx, programID, actorID); err != nil {
		return err
	}

	member, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("get member for delete: %w", err)
	}
	if member.ProgramID != programID {
		return domain.ErrProgramMemberNotFound
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program member: %w", err)
	}
	return nil
}

// ResendInvite signs a fresh invite token for an invited mentor and emails it again.
func (s *ProgramMemberService) ResendInvite(ctx context.Context, programID, id, actorID string) error {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.ResendInvite")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID), attribute.String("member.id", id), attribute.String("actor.id", actorID))

	if err := s.assertActiveProgramAdmin(ctx, programID, actorID); err != nil {
		span.RecordError(err)
		return err
	}
	if s.inviteSecret == "" {
		return fmt.Errorf("%w: mentor invites are not configured", domain.ErrUpstreamUnavailable)
	}

	member, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("get member for resend: %w", err)
	}
	if member.ProgramID != programID {
		return domain.ErrProgramMemberNotFound
	}
	if member.MemberType != models.MemberTypeMentor || member.Status == nil || *member.Status != models.ProgramMemberStatusInvited {
		return fmt.Errorf("%w: only an invited mentor can be sent the invite again", domain.ErrInvalidStateTransition)
	}
	// FR-018 / FR-023: invites are only for a published program.
	prog, err := s.programRepo.GetByID(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("get program for resend: %w", err)
	}
	if prog.Status != models.ProgramStatusPublished {
		return fmt.Errorf("%w: program must be published to resend an invite", domain.ErrInvalidStateTransition)
	}

	token, err := auth.GenerateInviteToken(programID, member.UserID, s.inviteSecret)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("generate invite token: %w", err)
	}
	s.notifier.NotifyMentorInvited(ctx, programID, member.UserID, token)
	return nil
}
