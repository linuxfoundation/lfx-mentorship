// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var programTermSvcTracer = otel.Tracer("program-terms-service")

// ProgramTermService orchestrates program term reads and writes.
type ProgramTermService struct {
	repo    domain.ProgramTermRepository
	appRepo domain.ApplicationRepository
}

// maxOpenTermsPerProgram is the maximum number of concurrently open terms allowed (FR-003).
const maxOpenTermsPerProgram = 4

// NewProgramTermService returns a ProgramTermService.
func NewProgramTermService(repo domain.ProgramTermRepository, appRepo domain.ApplicationRepository) *ProgramTermService {
	return &ProgramTermService{repo: repo, appRepo: appRepo}
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

// GetByProgramAndID returns the program term only when it belongs to the given parent program.
func (s *ProgramTermService) GetByProgramAndID(ctx context.Context, programID, id string) (*models.ProgramTerm, error) {
	ctx, span := programTermSvcTracer.Start(ctx, "ProgramTermService.GetByProgramAndID")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID), attribute.String("term.id", id))

	t, err := s.repo.GetByProgramAndID(ctx, programID, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program term for program: %w", err)
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

func (s *ProgramTermService) ListManagementByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTermManagementRow, *models.PaginationMeta, error) {
	return s.repo.ListManagementByProgram(ctx, programID, filter)
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
	if err := validateTermDates(input.StartDateTime, input.EndDateTime, input.ApplicationStartDate, input.ApplicationEndDate); err != nil {
		return nil, err
	}
	// Terms may only be created open or closed; "deleted" is reachable via Update.
	if input.Status == "" {
		input.Status = models.ProgramTermStatusOpen
	} else if input.Status != models.ProgramTermStatusOpen && input.Status != models.ProgramTermStatusClosed {
		return nil, fmt.Errorf("%w: status must be open or closed", domain.ErrInvalidInput)
	}

	// Open-term cap guard.
	if input.Status == models.ProgramTermStatusOpen {
		count, err := s.repo.CountOpenTermsByProgram(ctx, input.ProgramID)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("check open terms: %w", err)
		}
		if count >= maxOpenTermsPerProgram {
			return nil, fmt.Errorf("%w: program already has %d open term(s) (max %d)", domain.ErrStateLocked, count, maxOpenTermsPerProgram)
		}
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
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get term for update: %w", err)
	}
	if current.Status == models.ProgramTermStatusClosed && current.EndDateTime != nil && time.Now().After(*current.EndDateTime) {
		return nil, fmt.Errorf("%w: historical closed terms cannot be edited", domain.ErrStateLocked)
	}

	if input.Status != nil {
		return nil, fmt.Errorf("%w: use dedicated close or reopen routes for term lifecycle changes", domain.ErrInvalidInput)
	}
	start, end := current.StartDateTime, current.EndDateTime
	applicationStart, applicationEnd := current.ApplicationStartDate, current.ApplicationEndDate
	if input.StartDateTime != nil {
		start = input.StartDateTime
	}
	if input.EndDateTime != nil {
		end = input.EndDateTime
	}
	if input.ApplicationStartDate != nil {
		applicationStart = input.ApplicationStartDate
	}
	if input.ApplicationEndDate != nil {
		applicationEnd = input.ApplicationEndDate
	}
	if err := validateTermDates(start, end, applicationStart, applicationEnd); err != nil {
		return nil, err
	}

	t, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program term: %w", err)
	}
	return t, nil
}

// Close declines pending applications and closes an open term atomically.
func (s *ProgramTermService) Close(ctx context.Context, id string) (*models.ProgramTerm, int, error) {
	ctx, span := programTermSvcTracer.Start(ctx, "ProgramTermService.Close")
	defer span.End()
	term, declined, err := s.repo.CloseWithBulkDecline(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, 0, fmt.Errorf("close program term: %w", err)
	}
	return term, declined, nil
}

// Reopen returns a closed term to open status while applying the existing
// end-date and open-term-cap guards.
func (s *ProgramTermService) Reopen(ctx context.Context, id string) (*models.ProgramTerm, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get term for reopen: %w", err)
	}
	if current.Status != models.ProgramTermStatusClosed {
		return nil, fmt.Errorf("%w: only closed terms can be reopened", domain.ErrInvalidStateTransition)
	}
	if current.EndDateTime != nil && !time.Now().Before(*current.EndDateTime) {
		return nil, fmt.Errorf("%w: ended terms cannot be reopened", domain.ErrStateLocked)
	}
	count, err := s.repo.CountOpenTermsByProgram(ctx, current.ProgramID)
	if err != nil {
		return nil, fmt.Errorf("check open terms for reopen: %w", err)
	}
	if count >= maxOpenTermsPerProgram {
		return nil, fmt.Errorf("%w: program already has %d open term(s) (max %d)", domain.ErrStateLocked, count, maxOpenTermsPerProgram)
	}
	status := models.ProgramTermStatusOpen
	term, err := s.repo.Update(ctx, id, models.ProgramTermUpdateInput{Status: &status})
	if err != nil {
		return nil, fmt.Errorf("reopen program term: %w", err)
	}
	return term, nil
}

func validateTermDates(start, end, applicationStart, applicationEnd *time.Time) error {
	if start == nil || end == nil || !end.After(*start) {
		return fmt.Errorf("%w: term end date must be after start date", domain.ErrInvalidInput)
	}
	if applicationStart == nil || applicationEnd == nil || !applicationEnd.After(*applicationStart) || !applicationEnd.Before(*start) {
		return fmt.Errorf("%w: application window must end after it starts and before the term", domain.ErrInvalidInput)
	}
	return nil
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
