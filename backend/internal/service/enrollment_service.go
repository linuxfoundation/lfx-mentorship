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

var enrollmentSvcTracer = otel.Tracer("enrollments-service")

// EnrollmentService orchestrates enrollment reads and writes.
type EnrollmentService struct {
	repo domain.EnrollmentRepository
}

// NewEnrollmentService returns an EnrollmentService.
func NewEnrollmentService(repo domain.EnrollmentRepository) *EnrollmentService {
	return &EnrollmentService{repo: repo}
}

var validEnrollmentStatuses = map[string]bool{
	"active":    true,
	"graduated": true,
	"withdrawn": true,
	"hold":      true,
}

// GetByID returns the enrollment with the given ID.
func (s *EnrollmentService) GetByID(ctx context.Context, id string) (*models.Enrollment, error) {
	ctx, span := enrollmentSvcTracer.Start(ctx, "EnrollmentService.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("enrollment.id", id))

	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get enrollment: %w", err)
	}
	return e, nil
}

// ListByProgramTerm returns paginated enrollments for a program term.
func (s *EnrollmentService) ListByProgramTerm(ctx context.Context, programTermID string, filter models.EnrollmentFilter) ([]*models.Enrollment, *models.PaginationMeta, error) {
	ctx, span := enrollmentSvcTracer.Start(ctx, "EnrollmentService.ListByProgramTerm")
	defer span.End()
	span.SetAttributes(attribute.String("term.id", programTermID))

	enrollments, meta, err := s.repo.ListByProgramTerm(ctx, programTermID, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list enrollments: %w", err)
	}
	return enrollments, meta, nil
}

// Create validates input and creates an enrollment.
func (s *EnrollmentService) Create(ctx context.Context, programTermID string, input models.EnrollmentCreateInput) (*models.Enrollment, error) {
	ctx, span := enrollmentSvcTracer.Start(ctx, "EnrollmentService.Create")
	defer span.End()

	if input.MenteeUserID == "" {
		return nil, fmt.Errorf("%w: mentee_user_id is required", domain.ErrInvalidInput)
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if !validEnrollmentStatuses[input.Status] {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, input.Status)
	}
	input.ID = uuid.New().String()

	e, err := s.repo.Create(ctx, programTermID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create enrollment: %w", err)
	}
	return e, nil
}

// Update applies changes to an enrollment.
func (s *EnrollmentService) Update(ctx context.Context, id string, input models.EnrollmentUpdateInput) (*models.Enrollment, error) {
	ctx, span := enrollmentSvcTracer.Start(ctx, "EnrollmentService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("enrollment.id", id))

	if input.Status != nil && !validEnrollmentStatuses[*input.Status] {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}

	e, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update enrollment: %w", err)
	}
	return e, nil
}
