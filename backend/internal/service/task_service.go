// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
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

// GetByIDForActor returns one task when the actor is the assignee or an active reviewer.
func (s *TaskService) GetByIDForActor(ctx context.Context, id, actorID string) (*models.Task, error) {
	if actorID == "" {
		return nil, fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}
	t, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.AssigneeID == actorID {
		return t, nil
	}
	if err := s.assertReviewer(ctx, t, actorID); err != nil {
		return nil, err
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

// ListByApplicationForActor returns tasks constrained by actor privileges.
func (s *TaskService) ListByApplicationForActor(ctx context.Context, applicationID string, filter models.TaskFilter, actorID string) ([]*models.Task, *models.PaginationMeta, error) {
	if actorID == "" {
		return nil, nil, fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}
	app, err := s.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return nil, nil, fmt.Errorf("get application for task list access check: %w", err)
	}
	if app.UserID != actorID {
		task := &models.Task{ApplicationID: &applicationID}
		if err := s.assertReviewer(ctx, task, actorID); err != nil {
			return nil, nil, err
		}
	}
	return s.ListByApplication(ctx, applicationID, filter)
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

// ListByProgramTermForActor returns term tasks constrained by actor privileges.
func (s *TaskService) ListByProgramTermForActor(ctx context.Context, programTermID string, filter models.TaskFilter, actorID string) ([]*models.Task, *models.PaginationMeta, error) {
	if actorID == "" {
		return nil, nil, fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}
	term, err := s.termRepo.GetByID(ctx, programTermID)
	if err != nil {
		return nil, nil, fmt.Errorf("get term for task list access check: %w", err)
	}
	if auth.IsGatewayPrincipal(ctx) {
		return s.ListByProgramTerm(ctx, programTermID, filter)
	}
	_, err = s.memberRepo.FindActiveReviewerByProgramAndUser(ctx, term.ProgramID, actorID)
	if err != nil {
		if !errors.Is(err, domain.ErrProgramMemberNotFound) {
			return nil, nil, fmt.Errorf("find program member for term task list access check: %w", err)
		}
		filter.AssigneeID = actorID
		return s.ListByProgramTerm(ctx, programTermID, filter)
	}
	// A matching active reviewer membership leaves the caller's filter intact.
	return s.ListByProgramTerm(ctx, programTermID, filter)
}

// Create validates input and creates a task linked to an application.
func (s *TaskService) Create(ctx context.Context, applicationID string, input models.TaskCreateInput) (*models.Task, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.Create")
	defer span.End()

	if input.AssigneeID == "" {
		return nil, fmt.Errorf("%w: assignee_id is required", domain.ErrInvalidInput)
	}
	application, err := s.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("get application for task creation: %w", err)
	}
	if application.Status != models.ApplicationStatusAccepted || application.Role != models.ApplicationRoleMentee || application.UserID != input.AssigneeID {
		return nil, fmt.Errorf("%w: task assignee must be the accepted application's mentee", domain.ErrInvalidInput)
	}
	if input.Status == "" {
		input.Status = models.TaskStatusPending
	}
	if !input.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, input.Status)
	}
	if input.Category != nil && !input.Category.IsValid() {
		return nil, fmt.Errorf("%w: invalid category %q", domain.ErrInvalidInput, *input.Category)
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

	if input.Status != nil && !input.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}
	if input.ApplicationStatus != nil && !input.ApplicationStatus.IsValid() {
		return nil, fmt.Errorf("%w: invalid application status %q", domain.ErrInvalidInput, *input.ApplicationStatus)
	}
	if input.ProgramTermStatus != nil && !input.ProgramTermStatus.IsValid() {
		return nil, fmt.Errorf("%w: invalid program term status %q", domain.ErrInvalidInput, *input.ProgramTermStatus)
	}
	if input.Category != nil && !input.Category.IsValid() {
		return nil, fmt.Errorf("%w: invalid category %q", domain.ErrInvalidInput, *input.Category)
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
		effectiveFile := current.File
		if input.File != nil {
			effectiveFile = input.File
		}
		if next == models.TaskStatusSubmitted && current.SubmitFile != nil && *current.SubmitFile != "" && (effectiveFile == nil || *effectiveFile == "") {
			return nil, fmt.Errorf("%w: submitted tasks requiring a file must include file", domain.ErrInvalidInput)
		}

		// State transition guard: only incomplete (reset) is unrestricted direction-wise.
		if next != models.TaskStatusPending {
			var validTransition bool
			switch current.Status {
			case models.TaskStatusPending, models.TaskStatus("incomplete"):
				validTransition = next == models.TaskStatusInProgress
			case models.TaskStatusInProgress:
				validTransition = next == models.TaskStatusSubmitted
			case models.TaskStatusSubmitted:
				validTransition = next == models.TaskStatusCompleted
			}
			if !validTransition {
				return nil, fmt.Errorf("%w: cannot transition task from %q to %q", domain.ErrInvalidStateTransition, current.Status, next)
			}
		}

		// Actor permission: mentee (assignee) may only advance; reviewer may complete or reset.
		switch next {
		case models.TaskStatusInProgress, models.TaskStatusSubmitted:
			if !isAssignee {
				return nil, fmt.Errorf("%w: only the task assignee may mark it %s", domain.ErrForbidden, next)
			}
		case models.TaskStatusCompleted, models.TaskStatusPending:
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
	if input.Status != nil && (*input.Status == models.TaskStatusCompleted || *input.Status == models.TaskStatusSubmitted) && t.ApplicationID != nil {
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
	if auth.IsGatewayPrincipal(ctx) {
		return nil
	}
	programTermID := ""
	if task.ApplicationID != nil {
		app, err := s.appRepo.GetByID(ctx, *task.ApplicationID)
		if err == nil {
			programTermID = app.ProgramTermID
		} else {
			// Permit cleanup of orphaned tasks by falling back to the task's own term linkage.
			if errors.Is(err, domain.ErrApplicationNotFound) && task.ProgramTermID != nil && *task.ProgramTermID != "" {
				programTermID = *task.ProgramTermID
			} else {
				return fmt.Errorf("get application for reviewer check: %w", err)
			}
		}
	} else if task.ProgramTermID != nil {
		programTermID = *task.ProgramTermID
	} else {
		return fmt.Errorf("%w: task has no application or program term; cannot verify reviewer role", domain.ErrForbidden)
	}
	term, err := s.termRepo.GetByID(ctx, programTermID)
	if err != nil {
		return fmt.Errorf("get term for reviewer check: %w", err)
	}
	_, err = s.memberRepo.FindActiveReviewerByProgramAndUser(ctx, term.ProgramID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrProgramMemberNotFound) {
			return fmt.Errorf("%w: actor is not a member of this program", domain.ErrForbidden)
		}
		return fmt.Errorf("find program member for reviewer check: %w", err)
	}
	return nil
}

// Delete removes a task. Only an active mentor or program admin in the
// owning program may delete; assignees cannot delete their own tasks.
func (s *TaskService) Delete(ctx context.Context, id string, actorID string) error {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", id))
	if actorID == "" {
		return fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}

	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("get task for delete permission check: %w", err)
	}
	if current.AssigneeID == actorID {
		return fmt.Errorf("%w: task assignee cannot delete task", domain.ErrForbidden)
	}
	if err := s.assertReviewer(ctx, current, actorID); err != nil {
		span.RecordError(err)
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}
