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
	repo       domain.TaskRepository
	appRepo    domain.ApplicationRepository
	termRepo   domain.ProgramTermRepository
	memberRepo domain.ProgramMemberRepository
	notifier   domain.Notifier
}

// NewTaskService returns a TaskService.
func NewTaskService(
	repo domain.TaskRepository,
	appRepo domain.ApplicationRepository,
	termRepo domain.ProgramTermRepository,
	memberRepo domain.ProgramMemberRepository,
	notifier domain.Notifier,
) *TaskService {
	return &TaskService{repo: repo, appRepo: appRepo, termRepo: termRepo, memberRepo: memberRepo, notifier: notifier}
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

// ListByApplication returns paginated tasks for an application.
func (s *TaskService) ListByApplication(ctx context.Context, applicationID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.ListByApplication")
	defer span.End()
	span.SetAttributes(attribute.String("application.id", applicationID))

	tasks, meta, err := s.repo.ListByApplication(ctx, applicationID, filter)
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

// Create validates input and creates a task linked to an application.
func (s *TaskService) Create(ctx context.Context, applicationID string, input models.TaskCreateInput) (*models.Task, error) {
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

	t, err := s.repo.Create(ctx, applicationID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

// Update applies changes to a task.
// Permission rules:
//   - Only the task's assignee may mark it complete/submitted.
//   - When all prerequisite tasks for the application are complete, the admin is notified.
func (s *TaskService) Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", id))

	if input.Status != nil && !validTaskStatuses[*input.Status] {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}

	// FR-033: enforce state transitions and actor permissions when ActorID is known.
	if input.Status != nil && input.ActorID != "" {
		current, err := s.repo.GetByID(ctx, id)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("get task for permission check: %w", err)
		}
		isAssignee := current.AssigneeID == input.ActorID
		next := *input.Status

		// State transition guard: only incomplete (reset) is unrestricted direction-wise.
		if next != "incomplete" {
			var validTransition bool
			switch current.Status {
			case "incomplete":
				validTransition = next == "in_progress"
			case "in_progress":
				validTransition = next == "submitted"
			case "submitted":
				validTransition = next == "complete"
			}
			if !validTransition {
				return nil, fmt.Errorf("%w: cannot transition task from %q to %q", domain.ErrInvalidStateTransition, current.Status, next)
			}
		}

		// Actor permission: mentee (assignee) may only advance; reviewer may complete or reset.
		switch next {
		case "in_progress", "submitted":
			if !isAssignee {
				return nil, fmt.Errorf("%w: only the task assignee may mark it %s", domain.ErrForbidden, next)
			}
		case "complete", "incomplete":
			if isAssignee {
				return nil, fmt.Errorf("%w: only a reviewer may mark a task %s", domain.ErrForbidden, next)
			}
			// Principle VII-4: verify the actor holds an active mentor/admin role on this program.
			if err := s.assertReviewer(ctx, current, input.ActorID); err != nil {
				span.RecordError(err)
				return nil, err
			}
		}
	}

	t, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update task: %w", err)
	}

	// tasks_submitted side-effect (FR-034): if all prerequisite tasks are now
	// submitted or complete, mark the application and notify the admin.
	if input.Status != nil && (*input.Status == "complete" || *input.Status == "submitted") && t.ApplicationID != nil {
		total, complete, countErr := s.repo.CountPrerequisiteTasksByApplication(ctx, *t.ApplicationID)
		if countErr == nil && total > 0 && total == complete {
			trueBool := true
			_, _ = s.appRepo.Update(ctx, *t.ApplicationID, models.ApplicationUpdateInput{TasksSubmitted: &trueBool})
			s.notifier.NotifyAdminTasksSubmitted(ctx, *t.ApplicationID)
		}
	}

	return t, nil
}

// assertReviewer verifies that actorID holds an active mentor or program_admin role
// on the program that owns the given task.
func (s *TaskService) assertReviewer(ctx context.Context, task *models.Task, actorID string) error {
	if task.ApplicationID == nil {
		return fmt.Errorf("%w: task has no application; cannot verify reviewer role", domain.ErrForbidden)
	}
	app, err := s.appRepo.GetByID(ctx, *task.ApplicationID)
	if err != nil {
		return fmt.Errorf("get application for reviewer check: %w", err)
	}
	term, err := s.termRepo.GetByID(ctx, app.ProgramTermID)
	if err != nil {
		return fmt.Errorf("get term for reviewer check: %w", err)
	}
	member, err := s.memberRepo.FindByProgramAndUser(ctx, term.ProgramID, actorID)
	if err != nil {
		return fmt.Errorf("%w: actor is not a member of this program", domain.ErrForbidden)
	}
	if member.MemberType != "mentor" && member.MemberType != "program_admin" {
		return fmt.Errorf("%w: actor must be mentor or program_admin to review tasks", domain.ErrForbidden)
	}
	if member.Status != nil && *member.Status != "active" {
		return fmt.Errorf("%w: actor's program membership is not active", domain.ErrForbidden)
	}
	return nil
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
