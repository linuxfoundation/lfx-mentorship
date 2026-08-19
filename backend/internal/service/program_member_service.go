// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var programMemberSvcTracer = otel.Tracer("program-members-service")

// ProgramMemberService orchestrates program member reads and writes.
type ProgramMemberService struct {
	repo domain.ProgramMemberRepository
}

// NewProgramMemberService returns a ProgramMemberService.
func NewProgramMemberService(repo domain.ProgramMemberRepository) *ProgramMemberService {
	return &ProgramMemberService{repo: repo}
}

var validMemberTypes = map[string]bool{
	"maintainer": true,
	"mentor":     true,
	"apprentice": true,
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
func (s *ProgramMemberService) Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.Create")
	defer span.End()

	if input.UserID == "" {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	}
	if !validMemberTypes[input.MemberType] {
		return nil, fmt.Errorf("%w: member_type must be maintainer, mentor, or apprentice", domain.ErrInvalidInput)
	}
	input.ID = uuid.New().String()

	m, err := s.repo.Create(ctx, programID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program member: %w", err)
	}
	return m, nil
}

// Update patches a program member.
func (s *ProgramMemberService) Update(ctx context.Context, id string, input models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("member.id", id))

	m, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program member: %w", err)
	}
	return m, nil
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

// ListAdminsByProgram returns all admins for a program.
func (s *ProgramMemberService) ListAdminsByProgram(ctx context.Context, programID string) ([]*models.ProgramAdmin, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.ListAdmins")
	defer span.End()

	admins, err := s.repo.ListAdminsByProgram(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list program admins: %w", err)
	}
	return admins, nil
}

// AddAdmin links a user profile to a program as admin.
func (s *ProgramMemberService) AddAdmin(ctx context.Context, programID string, input models.ProgramAdminCreateInput) (*models.ProgramAdmin, error) {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.AddAdmin")
	defer span.End()

	if input.UserProfileID == "" {
		return nil, fmt.Errorf("%w: user_profile_id is required", domain.ErrInvalidInput)
	}
	if input.Role == "" {
		return nil, fmt.Errorf("%w: role is required", domain.ErrInvalidInput)
	}

	a, err := s.repo.AddAdmin(ctx, programID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("add program admin: %w", err)
	}
	return a, nil
}

// DeleteAdmin removes a program admin entry.
func (s *ProgramMemberService) DeleteAdmin(ctx context.Context, adminID string) error {
	ctx, span := programMemberSvcTracer.Start(ctx, "ProgramMemberService.DeleteAdmin")
	defer span.End()

	if err := s.repo.DeleteAdmin(ctx, adminID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program admin: %w", err)
	}
	return nil
}
