// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var applicationSvcTracer = otel.Tracer("applications-service")

// ApplicationService orchestrates application reads and writes.
type ApplicationService struct {
	repo        domain.ApplicationRepository
	taskRepo    domain.TaskRepository
	termRepo    domain.ProgramTermRepository
	programRepo domain.ProgramRepository
	memberRepo  domain.ProgramMemberRepository
	notifier    domain.Notifier
}

// taskTemplate mirrors the shape stored in programs.task_templates JSONB.
type taskTemplate struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	SubmitFile  *string `json:"submitFile"`
	DueDate     *string `json:"dueDate"`
}

// NewApplicationService returns an ApplicationService.
func NewApplicationService(
	repo domain.ApplicationRepository,
	taskRepo domain.TaskRepository,
	termRepo domain.ProgramTermRepository,
	programRepo domain.ProgramRepository,
	memberRepo domain.ProgramMemberRepository,
	notifier domain.Notifier,
) *ApplicationService {
	return &ApplicationService{repo: repo, taskRepo: taskRepo, termRepo: termRepo, programRepo: programRepo, memberRepo: memberRepo, notifier: notifier}
}

func (s *ApplicationService) isActiveReviewer(ctx context.Context, programID, actorID string) (bool, error) {
	if auth.IsGatewayPrincipal(ctx) {
		return true, nil
	}
	_, err := s.memberRepo.FindActiveReviewerByProgramAndUser(ctx, programID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrProgramMemberNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("find program member: %w", err)
	}
	return true, nil
}

// applicationTransitions maps current → allowed next statuses.
var applicationTransitions = map[models.ApplicationStatus][]models.ApplicationStatus{
	models.ApplicationStatusPending: {
		models.ApplicationStatusAccepted,
		models.ApplicationStatusDeclined,
		models.ApplicationStatusHold,
		models.ApplicationStatusWithdrawn,
	},
	models.ApplicationStatusHold: {
		models.ApplicationStatusAccepted,
		models.ApplicationStatusDeclined,
		models.ApplicationStatusPending,
	},
	// "accepted" is the enrolled state: a mentee stays accepted for the whole
	// term and moves straight to "graduated" at the end. There is no separate
	// "active" applications.status — "active" survives only as a display label
	// (see models.MenteeStatus) and as a program_members.status value.
	models.ApplicationStatusAccepted: {
		models.ApplicationStatusGraduated,
		models.ApplicationStatusDeclined,
	},
	models.ApplicationStatusDeclined:  {models.ApplicationStatusPending}, // allow re-open by admin
	models.ApplicationStatusWithdrawn: {},
	models.ApplicationStatusGraduated: {},
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

// GetByIDForActor returns one application when the actor is the applicant or an active reviewer.
func (s *ApplicationService) GetByIDForActor(ctx context.Context, id, actorID string) (*models.Application, error) {
	if actorID == "" {
		return nil, fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}
	app, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if app.UserID == actorID {
		return app, nil
	}
	term, err := s.termRepo.GetByID(ctx, app.ProgramTermID)
	if err != nil {
		return nil, fmt.Errorf("get program term for application access check: %w", err)
	}
	allowed, err := s.isActiveReviewer(ctx, term.ProgramID, actorID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf("%w: actor is not allowed to access this application", domain.ErrForbidden)
	}
	return app, nil
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

func (s *ApplicationService) ListByProgram(ctx context.Context, programID string, filter models.ProgramApplicationFilter) ([]*models.ProgramApplicationRow, *models.PaginationMeta, error) {
	if filter.Type == "" {
		filter.Type = models.ProgramApplicationTypeAll
	}
	if !filter.Type.IsValid() {
		return nil, nil, fmt.Errorf("%w: type must be current, past, or all", domain.ErrInvalidInput)
	}
	if filter.Status != "" && !models.ApplicationStatus(filter.Status).IsValid() {
		return nil, nil, fmt.Errorf("%w: invalid application status", domain.ErrInvalidInput)
	}
	return s.repo.ListByProgram(ctx, programID, filter)
}

// ListByProgramTermForActor returns term applications constrained by actor privileges.
func (s *ApplicationService) ListByProgramTermForActor(ctx context.Context, programTermID string, filter models.ApplicationFilter, actorID string) ([]*models.Application, *models.PaginationMeta, error) {
	if actorID == "" {
		return nil, nil, fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}
	term, err := s.termRepo.GetByID(ctx, programTermID)
	if err != nil {
		return nil, nil, fmt.Errorf("get program term for application list access check: %w", err)
	}
	allowed, err := s.isActiveReviewer(ctx, term.ProgramID, actorID)
	if err != nil {
		return nil, nil, err
	}
	if !allowed {
		filter.UserID = actorID
	}
	return s.ListByProgramTerm(ctx, programTermID, filter)
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
// Guards enforced:
//   - The term must be within its application window.
//   - The user must not already have a pending/submitted application for the same term.
func (s *ApplicationService) Create(ctx context.Context, programTermID string, input models.ApplicationCreateInput) (*models.Application, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.Create")
	defer span.End()

	if input.UserID == "" {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	}
	if !input.Role.IsValid() {
		return nil, fmt.Errorf("%w: role must be mentor or mentee", domain.ErrInvalidInput)
	}
	if input.AttendanceType != nil && !input.AttendanceType.IsValid() {
		return nil, fmt.Errorf("%w: attendance_type must be full_time or part_time", domain.ErrInvalidInput)
	}
	input.Status = models.ApplicationStatusPending // applications always start as pending

	// Application window guard (FR-016): term must be open and now within the window.
	term, err := s.termRepo.GetByID(ctx, programTermID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program term: %w", err)
	}
	if term.Status != models.ProgramTermStatusOpen {
		return nil, fmt.Errorf("%w: applications are not open for this term", domain.ErrIneligible)
	}
	now := time.Now()
	if term.ApplicationStartDate != nil && now.Before(*term.ApplicationStartDate) {
		return nil, fmt.Errorf("%w: application window has not opened yet", domain.ErrIneligible)
	}
	if term.ApplicationEndDate != nil && now.After(*term.ApplicationEndDate) {
		return nil, fmt.Errorf("%w: application window has closed", domain.ErrIneligible)
	}

	// Reapply guard: no existing non-terminal application for this term+user.
	existing, err := s.repo.FindByTermAndUser(ctx, programTermID, input.UserID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("check duplicate application: %w", err)
	}
	// Reapply guard (FR-030): allow only from withdrawn, never from declined.
	if existing != nil {
		if existing.Status == models.ApplicationStatusDeclined {
			return nil, fmt.Errorf("%w: reapplication is not permitted from a declined application", domain.ErrConflict)
		}
		if existing.Status != models.ApplicationStatusWithdrawn {
			return nil, fmt.Errorf("%w: an application for this term already exists (status: %s)", domain.ErrConflict, existing.Status)
		}
		input.ID = uuid.New().String()
		tasks, err := s.prerequisiteTasks(ctx, programTermID, input, term.ProgramID)
		if err != nil {
			return nil, fmt.Errorf("prepare reapplication prerequisite tasks: %w", err)
		}
		a, err := s.repo.ReapplyWithTasks(ctx, existing.ID, programTermID, input, tasks)
		if err != nil {
			return nil, fmt.Errorf("reapply application: %w", err)
		}
		return a, nil
	}

	input.ID = uuid.New().String()
	tasks, err := s.prerequisiteTasks(ctx, programTermID, input, term.ProgramID)
	if err != nil {
		return nil, fmt.Errorf("prepare prerequisite tasks: %w", err)
	}
	a, err := s.repo.CreateWithTasks(ctx, programTermID, input, tasks)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create application: %w", err)
	}

	return a, nil
}

func (s *ApplicationService) prerequisiteTasks(ctx context.Context, programTermID string, input models.ApplicationCreateInput, programID string) ([]models.TaskCreateInput, error) {
	prog, err := s.programRepo.GetByID(ctx, programID)
	if err != nil {
		return nil, fmt.Errorf("get program task templates: %w", err)
	}
	if len(prog.TaskTemplates) == 0 {
		return nil, nil
	}
	var templates []taskTemplate
	if err := json.Unmarshal(prog.TaskTemplates, &templates); err != nil {
		return nil, fmt.Errorf("decode program task templates: %w", err)
	}
	tasks := make([]models.TaskCreateInput, 0, len(templates))
	for _, tmpl := range templates {
		category := models.TaskCategoryPrerequisite
		termID := programTermID
		createdBy := input.UserID
		name := tmpl.Name
		tasks = append(tasks, models.TaskCreateInput{
			ID:            uuid.New().String(),
			ProgramTermID: &termID,
			AssigneeID:    input.UserID,
			Name:          &name,
			Description:   tmpl.Description,
			SubmitFile:    tmpl.SubmitFile,
			Category:      &category,
			Status:        models.TaskStatusIncomplete,
			CreatedBy:     &createdBy,
		})
	}
	return tasks, nil
}

// Update applies status changes to an application.
// Enforces the state machine defined in applicationTransitions.
// When accepting, attendance_type is required.
// When accepting, the mentee is notified.
func (s *ApplicationService) Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("application.id", id))

	if input.Status != nil && !input.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}
	if input.AttendanceType != nil && !input.AttendanceType.IsValid() {
		return nil, fmt.Errorf("%w: attendance_type must be full_time or part_time", domain.ErrInvalidInput)
	}
	if input.ProgramTermStatus != nil && !input.ProgramTermStatus.IsValid() {
		return nil, fmt.Errorf("%w: invalid program term status %q", domain.ErrInvalidInput, *input.ProgramTermStatus)
	}

	if input.Status != nil || input.ReviewerNote != nil || input.Evaluation != nil {
		current, err := s.repo.GetByID(ctx, id)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("get application for update: %w", err)
		}

		if input.ActorID == "" {
			return nil, fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
		}
		if input.Status == nil {
			if err := s.requireReviewer(ctx, current, input.ActorID); err != nil {
				return nil, err
			}
		} else {
			next := *input.Status
			if next != models.ApplicationStatusWithdrawn || input.ActorID != current.UserID {
				if err := s.requireReviewer(ctx, current, input.ActorID); err != nil {
					return nil, err
				}
			}
			allowed := applicationTransitions[current.Status]
			ok := false
			for _, candidate := range allowed {
				if candidate == next {
					ok = true
					break
				}
			}
			if !ok {
				return nil, fmt.Errorf("%w: cannot transition application from %q to %q", domain.ErrInvalidStateTransition, current.Status, next)
			}

			// Withdrawal guard: only the applicant may self-withdraw.
			if next == models.ApplicationStatusWithdrawn && input.ActorID != "" && current.UserID != input.ActorID {
				return nil, fmt.Errorf("%w: only the applicant may withdraw their application", domain.ErrForbidden)
			}

			// Accept guard: attendance_type is required when accepting.
			if next == models.ApplicationStatusAccepted {
				attType := input.AttendanceType
				if attType == nil {
					attType = current.AttendanceType
				}
				if attType == nil || *attType == "" {
					return nil, fmt.Errorf("%w: attendance_type is required when accepting an application", domain.ErrInvalidInput)
				}
			}
		}
	}

	a, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update application: %w", err)
	}

	// Post-update side-effects.
	if input.Status != nil {
		switch *input.Status {
		case models.ApplicationStatusAccepted:
			if a.Role == models.ApplicationRoleMentee {
				attType := ""
				if a.AttendanceType != nil {
					attType = string(*a.AttendanceType)
				}
				s.notifier.NotifyMenteeAccepted(ctx, id, attType)
			}
		}
	}

	return a, nil
}

func (s *ApplicationService) requireReviewer(ctx context.Context, application *models.Application, actorID string) error {
	if actorID == "" {
		return fmt.Errorf("%w: reviewer identity is required", domain.ErrForbidden)
	}
	term, err := s.termRepo.GetByID(ctx, application.ProgramTermID)
	if err != nil {
		return fmt.Errorf("get program term for reviewer check: %w", err)
	}
	allowed, err := s.isActiveReviewer(ctx, term.ProgramID, actorID)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%w: actor is not an active program reviewer", domain.ErrForbidden)
	}
	return nil
}

// Delete removes an application and its authorization descendants.
func (s *ApplicationService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete application: %w", err)
	}
	return nil
}

// WithdrawForMentee performs a staff-assisted withdrawal authorized by the
// active program admin of the application's parent program.
func (s *ApplicationService) WithdrawForMentee(ctx context.Context, id, actorID string) (*models.Application, error) {
	if actorID == "" {
		return nil, fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}
	application, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get application for assisted withdrawal: %w", err)
	}
	term, err := s.termRepo.GetByID(ctx, application.ProgramTermID)
	if err != nil {
		return nil, fmt.Errorf("get application term: %w", err)
	}
	var memberErr error
	if !auth.IsGatewayPrincipal(ctx) {
		_, memberErr = s.memberRepo.FindActiveProgramAdminByProgramAndUser(ctx, term.ProgramID, actorID)
	}
	if memberErr != nil {
		if errors.Is(memberErr, domain.ErrProgramMemberNotFound) {
			return nil, fmt.Errorf("%w: actor must be an active program_admin", domain.ErrForbidden)
		}
		return nil, fmt.Errorf("verify program admin: %w", memberErr)
	}
	status := models.ApplicationStatusWithdrawn
	updated, err := s.repo.Update(ctx, id, models.ApplicationUpdateInput{Status: &status})
	if err != nil {
		return nil, fmt.Errorf("assisted withdrawal: %w", err)
	}
	return updated, nil
}

// WithdrawForMenteeAfterGatewayAuthorization applies the state transition after
// Heimdall has verified the caller's manager relation on the application.
func (s *ApplicationService) WithdrawForMenteeAfterGatewayAuthorization(ctx context.Context, id string) (*models.Application, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, fmt.Errorf("get application for assisted withdrawal: %w", err)
	}
	status := models.ApplicationStatusWithdrawn
	updated, err := s.repo.Update(ctx, id, models.ApplicationUpdateInput{Status: &status})
	if err != nil {
		return nil, fmt.Errorf("assisted withdrawal: %w", err)
	}
	return updated, nil
}

// BulkDeclineByTerm moves all pending/submitted applications in a term to declined.
func (s *ApplicationService) BulkDeclineByTerm(ctx context.Context, termID string) (int, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.BulkDeclineByTerm")
	defer span.End()
	span.SetAttributes(attribute.String("term.id", termID))

	count, err := s.repo.BulkDeclineByTerm(ctx, termID)
	if err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("bulk decline: %w", err)
	}
	return count, nil
}

// ListPastMenteesByTerm returns accepted/graduated applications for a term.
func (s *ApplicationService) ListPastMenteesByTerm(ctx context.Context, termID string) ([]*models.Application, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.ListPastMenteesByTerm")
	defer span.End()
	span.SetAttributes(attribute.String("term.id", termID))

	apps, err := s.repo.ListPastMenteesByTerm(ctx, termID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list past mentees: %w", err)
	}
	return apps, nil
}
