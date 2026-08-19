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

var programTermSvcTracer = otel.Tracer("program-terms-service")

// ProgramTermService orchestrates program term reads and writes.
type ProgramTermService struct {
	repo domain.ProgramTermRepository
}

// NewProgramTermService returns a ProgramTermService.
func NewProgramTermService(repo domain.ProgramTermRepository) *ProgramTermService {
	return &ProgramTermService{repo: repo}
}

// GetByID returns the program term with the given ID.
func (s *ProgramTermService) GetByID(ctx context.Context, id string) (*models.ProgramTerm, error) {
	ctx, span := programTermSvcTracer.Start(ctx, "ProgramTermService.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("term.id", id))

	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program term: %w", err)
	}
	return t, nil
}

// ListByProgram returns paginated terms for a program.
func (s *ProgramTermService) ListByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error) {
	ctx, span := programTermSvcTracer.Start(ctx, "ProgramTermService.ListByProgram")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID))

	terms, meta, err := s.repo.ListByProgram(ctx, programID, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list program terms: %w", err)
	}
	return terms, meta, nil
}

// Create validates input and creates a program term.
func (s *ProgramTermService) Create(ctx context.Context, input models.ProgramTermCreateInput) (*models.ProgramTerm, error) {
	ctx, span := programTermSvcTracer.Start(ctx, "ProgramTermService.Create")
	defer span.End()

	if strings.TrimSpace(input.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrInvalidInput)
	}
	if input.ProgramID == "" {
		return nil, fmt.Errorf("%w: program_id is required", domain.ErrInvalidInput)
	}
	if input.Status != "open" && input.Status != "closed" {
		input.Status = "open"
	}
	input.ID = uuid.New().String()

	t, err := s.repo.Create(ctx, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program term: %w", err)
	}
	return t, nil
}

// Update applies changes to the program term with the given ID.
func (s *ProgramTermService) Update(ctx context.Context, id string, input models.ProgramTermUpdateInput) (*models.ProgramTerm, error) {
	ctx, span := programTermSvcTracer.Start(ctx, "ProgramTermService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("term.id", id))

	if input.Status != nil && *input.Status != "open" && *input.Status != "closed" {
		return nil, fmt.Errorf("%w: status must be open or closed", domain.ErrInvalidInput)
	}

	t, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program term: %w", err)
	}
	return t, nil
}

// Delete removes the program term with the given ID.
func (s *ProgramTermService) Delete(ctx context.Context, id string) error {
	ctx, span := programTermSvcTracer.Start(ctx, "ProgramTermService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("term.id", id))

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program term: %w", err)
	}
	return nil
}
