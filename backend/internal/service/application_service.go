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

var applicationSvcTracer = otel.Tracer("applications-service")

// ApplicationService orchestrates application reads and writes.
type ApplicationService struct {
	repo domain.ApplicationRepository
}

// NewApplicationService returns an ApplicationService.
func NewApplicationService(repo domain.ApplicationRepository) *ApplicationService {
	return &ApplicationService{repo: repo}
}

var validApplicationRoles = map[string]bool{
	"mentor": true,
	"mentee": true,
}

var validApplicationStatuses = map[string]bool{
	"pending":   true,
	"accepted":  true,
	"declined":  true,
	"withdrawn": true,
}

// GetByID returns the application with the given ID.
func (s *ApplicationService) GetByID(ctx context.Context, id string) (*models.Application, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("application.id", id))

	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get application: %w", err)
	}
	return a, nil
}

// ListByProgramTerm returns paginated applications for a program term.
func (s *ApplicationService) ListByProgramTerm(ctx context.Context, programTermID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.ListByProgramTerm")
	defer span.End()
	span.SetAttributes(attribute.String("term.id", programTermID))

	apps, meta, err := s.repo.ListByProgramTerm(ctx, programTermID, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list applications: %w", err)
	}
	return apps, meta, nil
}

// ListByUser returns paginated applications for a user.
func (s *ApplicationService) ListByUser(ctx context.Context, userID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.ListByUser")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))

	apps, meta, err := s.repo.ListByUser(ctx, userID, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list user applications: %w", err)
	}
	return apps, meta, nil
}

// Create validates input and creates an application.
func (s *ApplicationService) Create(ctx context.Context, programTermID string, input models.ApplicationCreateInput) (*models.Application, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.Create")
	defer span.End()

	if input.UserID == "" {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	}
	if !validApplicationRoles[input.Role] {
		return nil, fmt.Errorf("%w: role must be mentor or mentee", domain.ErrInvalidInput)
	}
	if input.Status == "" {
		input.Status = "pending"
	}
	input.ID = uuid.New().String()

	a, err := s.repo.Create(ctx, programTermID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create application: %w", err)
	}
	return a, nil
}

// Update applies status changes to an application.
func (s *ApplicationService) Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("application.id", id))

	if input.Status != nil && !validApplicationStatuses[*input.Status] {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}

	a, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update application: %w", err)
	}
	return a, nil
}

// Delete removes an application (withdrawal).
func (s *ApplicationService) Delete(ctx context.Context, id string) error {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("application.id", id))

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete application: %w", err)
	}
	return nil
}
