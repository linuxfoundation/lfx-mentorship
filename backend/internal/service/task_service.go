// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var taskSvcTracer = otel.Tracer("tasks-service")

// validateDueDate enforces the YYYY-MM-DD task due-date contract.
func validateDueDate(value *string) error {
	if value == nil {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, *value); err != nil {
		return fmt.Errorf("%w: due date must be a YYYY-MM-DD date", domain.ErrInvalidInput)
	}
	return nil
}

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
		input.Status = models.TaskStatusIncomplete
	}
	if !input.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, input.Status)
	}
	if input.Category != nil && !input.Category.IsValid() {
		return nil, fmt.Errorf("%w: invalid category %q", domain.ErrInvalidInput, *input.Category)
	}
	if err := validateDueDate(input.DueDate); err != nil {
		return nil, err
	}
	input.ID = uuid.New().String()

	t, err := s.repo.Create(ctx, applicationID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

// validateTaskUpdate checks the enum fields of a task update and refuses a direct file write.
func validateTaskUpdate(input models.TaskUpdateInput) error {
	if input.Status != nil && !input.Status.IsValid() {
		return fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}
	if input.ApplicationStatus != nil && !input.ApplicationStatus.IsValid() {
		return fmt.Errorf("%w: invalid application status %q", domain.ErrInvalidInput, *input.ApplicationStatus)
	}
	if input.ProgramTermStatus != nil && !input.ProgramTermStatus.IsValid() {
		return fmt.Errorf("%w: invalid program term status %q", domain.ErrInvalidInput, *input.ProgramTermStatus)
	}
	if input.Category != nil && !input.Category.IsValid() {
		return fmt.Errorf("%w: invalid category %q", domain.ErrInvalidInput, *input.Category)
	}
	return reservedFileField("file", input.File)
}

// Update applies the assignee's submission or a reviewer's review to a task.
// Permission rules:
//   - Only the task's assignee may mark it in_progress/submitted, moving forward one step.
//   - Only an active reviewer who is not the assignee may mark it complete, reset it to
//     incomplete, or set its denormalised application and program term statuses.
//   - When every prerequisite task is submitted or complete, the application is flagged
//     tasks_submitted and program admins are notified the first time only.
func (s *TaskService) Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", id))

	if err := validateTaskUpdate(input); err != nil {
		return nil, err
	}
	if err := validateDueDate(input.DueDate); err != nil {
		return nil, err
	}

	if input.ActorID != "" {
		if err := s.authorizeUpdate(ctx, id, input); err != nil {
			span.RecordError(err)
			return nil, err
		}
	}

	t, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update task: %w", err)
	}

	// tasks_submitted side-effect (FR-034).
	if input.Status != nil && (*input.Status == models.TaskStatusComplete || *input.Status == models.TaskStatusSubmitted) && t.ApplicationID != nil {
		s.markTasksSubmitted(ctx, *t.ApplicationID)
	}

	return t, nil
}

// authorizeUpdate enforces FR-033 for Update: the status transition guard and who may make it.
func (s *TaskService) authorizeUpdate(ctx context.Context, id string, input models.TaskUpdateInput) error {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get task for permission check: %w", err)
	}
	// A review that leaves status alone still sets the reviewer-owned denormalised statuses.
	if input.Status == nil {
		return s.assertNonAssigneeReviewer(ctx, current, input.ActorID)
	}
	next := *input.Status
	if next == models.TaskStatusSubmitted && current.SubmitFile != nil && *current.SubmitFile != "" && (current.File == nil || *current.File == "") {
		return fmt.Errorf("%w: upload the required file before submitting", domain.ErrInvalidInput)
	}

	// State transition guard: only incomplete (reset) is unrestricted direction-wise.
	if next != models.TaskStatusIncomplete {
		var validTransition bool
		switch current.Status {
		case models.TaskStatusIncomplete:
			validTransition = next == models.TaskStatusInProgress
		case models.TaskStatusInProgress:
			validTransition = next == models.TaskStatusSubmitted
		case models.TaskStatusSubmitted:
			validTransition = next == models.TaskStatusComplete
		}
		if !validTransition {
			return fmt.Errorf("%w: cannot transition task from %q to %q", domain.ErrInvalidStateTransition, current.Status, next)
		}
	}

	// Actor permission: mentee (assignee) may only advance; reviewer may complete or reset.
	switch next {
	case models.TaskStatusInProgress, models.TaskStatusSubmitted:
		if current.AssigneeID != input.ActorID {
			return fmt.Errorf("%w: only the task assignee may mark it %s", domain.ErrForbidden, next)
		}
	case models.TaskStatusComplete, models.TaskStatusIncomplete:
		return s.assertNonAssigneeReviewer(ctx, current, input.ActorID)
	}
	return nil
}

// Edit applies a reviewer's full edit of a task: any field, and any status from any status, with
// no transition guard. Only an active mentor or program admin of the task's program may edit, and
// never the task's own assignee, who goes through Update. An empty submit_file or due_date clears it.
func (s *TaskService) Edit(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error) {
	ctx, span := taskSvcTracer.Start(ctx, "TaskService.Edit")
	defer span.End()
	span.SetAttributes(attribute.String("task.id", id))

	if err := validateTaskUpdate(input); err != nil {
		return nil, err
	}
	if input.DueDate != nil && *input.DueDate != "" {
		if err := validateDueDate(input.DueDate); err != nil {
			return nil, err
		}
	}
	if input.ActorID == "" {
		return nil, fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}

	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get task for edit permission check: %w", err)
	}
	if err := s.assertNonAssigneeReviewer(ctx, current, input.ActorID); err != nil {
		span.RecordError(err)
		return nil, err
	}

	t, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("edit task: %w", err)
	}

	// FR-034: a new status or category can complete the prerequisite set. A move back out of
	// submitted or complete leaves tasks_submitted set: it records the first full submission.
	if (input.Status != nil || input.Category != nil) && t.ApplicationID != nil {
		s.markTasksSubmitted(ctx, *t.ApplicationID)
	}
	return t, nil
}

// markTasksSubmitted flags the application once every prerequisite task is submitted or
// complete, notifying program admins only on the first flip so later reviews do not re-send.
func (s *TaskService) markTasksSubmitted(ctx context.Context, applicationID string) {
	flipped, err := s.appRepo.MarkTasksSubmitted(ctx, applicationID)
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
		return
	}
	if flipped {
		s.notifier.NotifyAdminTasksSubmitted(ctx, applicationID)
	}
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

// assertNonAssigneeReviewer verifies that actorID is an active reviewer of the task's program
// and not the task's assignee, so no one reviews or edits their own task.
func (s *TaskService) assertNonAssigneeReviewer(ctx context.Context, task *models.Task, actorID string) error {
	if task.AssigneeID == actorID {
		return fmt.Errorf("%w: the task assignee cannot review or edit their own task", domain.ErrForbidden)
	}
	// Principle VII-4: verify the actor holds an active mentor/admin role on this program.
	return s.assertReviewer(ctx, task, actorID)
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
