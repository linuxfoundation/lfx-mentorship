// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var programSvcTracer = otel.Tracer("programs-service")

// ProgramService orchestrates program reads and writes.
type ProgramService struct {
	repo domain.ProgramRepository
}

// NewProgramService returns a ProgramService.
func NewProgramService(repo domain.ProgramRepository) *ProgramService {
	return &ProgramService{repo: repo}
}

// GetByID returns the program with the given ID.
func (s *ProgramService) GetByID(ctx context.Context, id string) (*models.Program, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", id))

	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program: %w", err)
	}
	return p, nil
}

// GetBySlug returns the program with the given slug.
func (s *ProgramService) GetBySlug(ctx context.Context, slug string) (*models.Program, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.GetBySlug")
	defer span.End()
	span.SetAttributes(attribute.String("program.slug", slug))

	p, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program by slug: %w", err)
	}
	return p, nil
}

// List returns a paginated list of programs.
func (s *ProgramService) List(ctx context.Context, filter models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.List")
	defer span.End()

	programs, meta, err := s.repo.List(ctx, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list programs: %w", err)
	}
	return programs, meta, nil
}

// Create validates input and creates a program.
func (s *ProgramService) Create(ctx context.Context, input models.ProgramCreateInput) (*models.Program, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.Create")
	defer span.End()

	if strings.TrimSpace(input.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(input.Slug) == "" {
		return nil, fmt.Errorf("%w: slug is required", domain.ErrInvalidInput)
	}
	if input.Status != "" && !input.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, input.Status)
	}
	if input.Status == "" {
		input.Status = models.ProgramStatusPending
	}
	input.ID = uuid.New().String()

	p, err := s.repo.Create(ctx, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program: %w", err)
	}
	return p, nil
}

// Update validates and applies changes to the program with the given ID.
func (s *ProgramService) Update(ctx context.Context, id string, input models.ProgramUpdateInput) (*models.Program, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", id))

	if input.Status != nil && !input.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}

	p, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program: %w", err)
	}
	return p, nil
}

// Delete removes the program with the given ID.
func (s *ProgramService) Delete(ctx context.Context, id string) error {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", id))

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program: %w", err)
	}
	return nil
}

// ListSkills returns all skills for a program.
func (s *ProgramService) ListSkills(ctx context.Context, programID string) ([]*models.ProgramSkill, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.ListSkills")
	defer span.End()

	skills, err := s.repo.ListSkills(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list skills: %w", err)
	}
	return skills, nil
}

// AddSkill adds a skill to a program.
func (s *ProgramService) AddSkill(ctx context.Context, programID string, input models.ProgramSkillCreateInput) (*models.ProgramSkill, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.AddSkill")
	defer span.End()

	if strings.TrimSpace(input.Skill) == "" {
		return nil, fmt.Errorf("%w: skill is required", domain.ErrInvalidInput)
	}

	skill, err := s.repo.AddSkill(ctx, programID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("add skill: %w", err)
	}
	return skill, nil
}

// DeleteSkill removes a skill from a program.
func (s *ProgramService) DeleteSkill(ctx context.Context, skillID string) error {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.DeleteSkill")
	defer span.End()

	if err := s.repo.DeleteSkill(ctx, skillID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete skill: %w", err)
	}
	return nil
}

// GetFundingStats returns funding stats for a program.
func (s *ProgramService) GetFundingStats(ctx context.Context, programID string) (*models.ProgramFundingStats, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.GetFundingStats")
	defer span.End()

	stats, err := s.repo.GetFundingStats(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get funding stats: %w", err)
	}
	return stats, nil
}

// ListInvitationTokens returns all invitation tokens for a program.
func (s *ProgramService) ListInvitationTokens(ctx context.Context, programID string) ([]*models.InvitationToken, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.ListInvitationTokens")
	defer span.End()

	tokens, err := s.repo.ListInvitationTokens(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list invitation tokens: %w", err)
	}
	return tokens, nil
}

// CreateInvitationToken creates an invitation token for a program.
func (s *ProgramService) CreateInvitationToken(ctx context.Context, programID string, input models.InvitationTokenCreateInput) (*models.InvitationToken, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.CreateInvitationToken")
	defer span.End()

	if input.Role != "mentor" && input.Role != "mentee" {
		return nil, fmt.Errorf("%w: role must be mentor or mentee", domain.ErrInvalidInput)
	}

	token, err := s.repo.CreateInvitationToken(ctx, programID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create invitation token: %w", err)
	}
	return token, nil
}

// DeleteInvitationToken removes an invitation token.
func (s *ProgramService) DeleteInvitationToken(ctx context.Context, tokenID string) error {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.DeleteInvitationToken")
	defer span.End()

	if err := s.repo.DeleteInvitationToken(ctx, tokenID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete invitation token: %w", err)
	}
	return nil
}
