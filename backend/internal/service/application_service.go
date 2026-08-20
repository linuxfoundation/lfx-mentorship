// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
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
	notifier domain.Notifier,
) *ApplicationService {
	return &ApplicationService{repo: repo, taskRepo: taskRepo, termRepo: termRepo, programRepo: programRepo, notifier: notifier}
}

// applicationTransitions maps current → allowed next statuses.
var applicationTransitions = map[string][]string{
	"pending":   {"accepted", "declined", "hold", "withdrawn"},
	"hold":      {"accepted", "declined", "pending"},
	"accepted":  {"active", "declined"},
	"active":    {"graduated", "declined"},
	"declined":  {"pending"}, // allow re-open by admin
	"withdrawn": {},
	"graduated": {},
}

var validApplicationRoles = map[string]bool{
	"mentor": true,
	"mentee": true,
}

var validApplicationStatuses = map[string]bool{
	"pending":   true,
	"accepted":  true,
	"active":    true,
	"declined":  true,
	"withdrawn": true,
	"graduated": true,
	"hold":      true,
}

var validAttendanceTypes = map[string]bool{
	"full_time": true,
	"part_time": true,
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
// Guards enforced:
//   - The term must be within its application window.
//   - The user must not already have a pending/submitted application for the same term.
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

	// Application window guard (FR-016): term must be open and now within the window.
	term, err := s.termRepo.GetByID(ctx, programTermID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program term: %w", err)
	}
	if term.Status != "open" {
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
		if existing.Status == "declined" {
			return nil, fmt.Errorf("%w: reapplication is not permitted from a declined application", domain.ErrConflict)
		}
		if existing.Status != "withdrawn" {
			return nil, fmt.Errorf("%w: an application for this term already exists (status: %s)", domain.ErrConflict, existing.Status)
		}
	}

	input.ID = uuid.New().String()
	a, err := s.repo.Create(ctx, programTermID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create application: %w", err)
	}

	// Task template cloning (FR-032): clone prerequisite tasks from the program.
	prog, progErr := s.programRepo.GetByID(ctx, term.ProgramID)
	if progErr == nil && len(prog.TaskTemplates) > 0 {
		var templates []taskTemplate
		if jsonErr := json.Unmarshal(prog.TaskTemplates, &templates); jsonErr == nil {
			for _, tmpl := range templates {
				cat := "prerequisite"
				status := "incomplete"
				custom := false
				termIDCopy := programTermID
				createdBy := input.UserID
				nameCopy := tmpl.Name
				_, _ = s.taskRepo.Create(ctx, a.ID, models.TaskCreateInput{
					ID:            uuid.New().String(),
					ProgramTermID: &termIDCopy,
					AssigneeID:    input.UserID,
					Name:          &nameCopy,
					Description:   tmpl.Description,
					SubmitFile:    tmpl.SubmitFile,
					Category:      &cat,
					Status:        status,
					Custom:        custom,
					CreatedBy:     &createdBy,
				})
			}
		}
	}

	return a, nil
}

// Update applies status changes to an application.
// Enforces the state machine defined in applicationTransitions.
// When accepting, attendance_type is required.
// When transitioning to active, if all prerequisite tasks are complete the admin is notified.
func (s *ApplicationService) Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("application.id", id))

	if input.Status != nil && !validApplicationStatuses[*input.Status] {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}
	if input.AttendanceType != nil && !validAttendanceTypes[*input.AttendanceType] {
		return nil, fmt.Errorf("%w: attendance_type must be full_time or part_time", domain.ErrInvalidInput)
	}

	if input.Status != nil {
		current, err := s.repo.GetByID(ctx, id)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("get application for update: %w", err)
		}

		next := *input.Status
		allowed := applicationTransitions[current.Status]
		ok := false
		for _, s := range allowed {
			if s == next {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("%w: cannot transition application from %q to %q", domain.ErrInvalidStateTransition, current.Status, next)
		}

		// Withdrawal guard: only the applicant may self-withdraw.
		if next == "withdrawn" && input.ActorID != "" && current.UserID != input.ActorID {
			return nil, fmt.Errorf("%w: only the applicant may withdraw their application", domain.ErrForbidden)
		}

		// Accept guard: attendance_type is required when accepting.
		if next == "accepted" {
			attType := input.AttendanceType
			if attType == nil {
				attType = current.AttendanceType
			}
			if attType == nil || *attType == "" {
				return nil, fmt.Errorf("%w: attendance_type is required when accepting an application", domain.ErrInvalidInput)
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
		case "accepted":
			attType := ""
			if a.AttendanceType != nil {
				attType = *a.AttendanceType
			}
			s.notifier.NotifyMenteeAccepted(ctx, id, attType)
		}
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

// ListPastMenteesByTerm returns accepted/active/graduated applications for a term.
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
