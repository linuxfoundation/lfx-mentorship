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

var taskSvcTracer = otel.Tracer("tasks-service")

// TaskService orchestrates task reads and writes.
type TaskService struct {
	repo domain.TaskRepository
}

// NewTaskService returns a TaskService.
func NewTaskService(repo domain.TaskRepository) *TaskService {
	return &TaskService{repo: repo}
}

var validTaskStatuses = map[string]bool{
	"incomplete":  true,
	"in_progress": true,
	"complete":    true,
	"submitted":   true,
}

// GetByID returns the task with the given ID.
func (s *TaskService) GetByID(ctx context.Context, id string) (*models.Task, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", id))

	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get task: %w", err)
	}
	return t, nil
}

// ListByEnrollment returns paginated tasks for an enrollment.
func (s *TaskService) ListByEnrollment(ctx context.Context, enrollmentID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.ListByEnrollment")
	defer span.End()
	span.SetAttributes(attribute.String("enrollment.id", enrollmentID))

	tasks, meta, err := s.repo.ListByEnrollment(ctx, enrollmentID, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, meta, nil
}

// ListByProgramTerm returns paginated tasks for a program term.
func (s *TaskService) ListByProgramTerm(ctx context.Context, programTermID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.ListByProgramTerm")
	defer span.End()
	span.SetAttributes(attribute.String("term.id", programTermID))

	tasks, meta, err := s.repo.ListByProgramTerm(ctx, programTermID, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list tasks by term: %w", err)
	}
	return tasks, meta, nil
}

// Create validates input and creates a task linked to an enrollment.
func (s *TaskService) Create(ctx context.Context, enrollmentID string, input models.TaskCreateInput) (*models.Task, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.Create")
	defer span.End()

	if input.AssigneeID == "" {
		return nil, fmt.Errorf("%w: assignee_id is required", domain.ErrInvalidInput)
	}
	if input.Status == "" {
		input.Status = "incomplete"
	}
	if !validTaskStatuses[input.Status] {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, input.Status)
	}
	input.ID = uuid.New().String()

	t, err := s.repo.Create(ctx, enrollmentID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

// Update applies changes to a task.
func (s *TaskService) Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", id))

	if input.Status != nil && !validTaskStatuses[*input.Status] {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}

	t, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update task: %w", err)
	}
	return t, nil
}

// Delete removes a task.
func (s *TaskService) Delete(ctx context.Context, id string) error {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", id))

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}
