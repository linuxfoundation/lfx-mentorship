// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	profileRepo domain.UserProfileRepository
	notifier    domain.Notifier
	access      domain.AccessWaiter
}

// taskTemplate mirrors the shape stored in programs.task_templates JSONB.
type taskTemplate struct {
	Name        string       `json:"name"`
	Description *string      `json:"description"`
	SubmitFile  *string      `json:"submitFile"`
	DueDate     *string      `json:"dueDate"`
	Custom      templateFlag `json:"custom"`
}

// templateFlag is a task template's custom flag. Templates imported before the migration
// normalized them store it as the legacy string "true", "false" or "undefined"; only "true" is set.
type templateFlag bool

func (f *templateFlag) UnmarshalJSON(data []byte) error {
	var value bool
	if err := json.Unmarshal(data, &value); err == nil {
		*f = templateFlag(value)
		return nil
	}
	var legacy string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*f = templateFlag(strings.EqualFold(strings.TrimSpace(legacy), "true"))
	return nil
}

// prerequisite returns the template as applications clone it.
func (t taskTemplate) prerequisite() models.PrerequisiteTemplate {
	var description *string
	if t.Description != nil && strings.TrimSpace(*t.Description) != "" {
		trimmed := strings.TrimSpace(*t.Description)
		description = &trimmed
	}
	return models.PrerequisiteTemplate{
		Name:        strings.TrimSpace(t.Name),
		Description: description,
		SubmitFile:  cloneSubmitFile(t.SubmitFile),
		DueDate:     cloneDueDate(t.DueDate),
		Custom:      bool(t.Custom),
	}
}

// cloneSubmitFile keeps a template's submitFile only when it is "required". Templates saved before
// the migration and update normalized them may hold "" or another value, cloned as no upload.
func cloneSubmitFile(value *string) *string {
	if value == nil || *value != taskSubmitFileRequired {
		return nil
	}
	return value
}

// cloneDueDate keeps a template's due date only when it is a YYYY-MM-DD date, as
// tasks_due_date_check requires; anything else is cloned as no due date, as the migration does.
func cloneDueDate(value *string) *string {
	if value == nil || validateDueDate(value) != nil {
		return nil
	}
	return value
}

// NewApplicationService returns an ApplicationService.
func NewApplicationService(
	repo domain.ApplicationRepository,
	taskRepo domain.TaskRepository,
	termRepo domain.ProgramTermRepository,
	programRepo domain.ProgramRepository,
	memberRepo domain.ProgramMemberRepository,
	profileRepo domain.UserProfileRepository,
	notifier domain.Notifier,
) *ApplicationService {
	return &ApplicationService{repo: repo, taskRepo: taskRepo, termRepo: termRepo, programRepo: programRepo, memberRepo: memberRepo, profileRepo: profileRepo, notifier: notifier}
}

// SetAccessWaiter makes Create respond only once the applicant can read the
// new application, or the waiter gives up.
func (s *ApplicationService) SetAccessWaiter(access domain.AccessWaiter) {
	s.access = access
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

// requireProgramAdmin refuses unless the actor is an active program admin of the term's program:
// the relation Heimdall checks on the status, delete, bulk-decline and assisted-withdrawal routes.
// A gateway principal has already passed that check, so it is not repeated.
func (s *ApplicationService) requireProgramAdmin(ctx context.Context, programTermID, actorID string) error {
	if auth.IsGatewayPrincipal(ctx) {
		return nil
	}
	if actorID == "" {
		return fmt.Errorf("%w: actor identity is required", domain.ErrForbidden)
	}
	term, err := s.termRepo.GetByID(ctx, programTermID)
	if err != nil {
		return fmt.Errorf("get program term for program admin check: %w", err)
	}
	if _, err := s.memberRepo.FindActiveProgramAdminByProgramAndUser(ctx, term.ProgramID, actorID); errors.Is(err, domain.ErrProgramMemberNotFound) {
		return fmt.Errorf("%w: actor must be an active program_admin", domain.ErrForbidden)
	} else if err != nil {
		return fmt.Errorf("verify program admin: %w", err)
	}
	return nil
}

// applicationTransitions maps current → allowed next statuses.
var applicationTransitions = map[models.ApplicationStatus][]models.ApplicationStatus{
	models.ApplicationStatusPending: {
		models.ApplicationStatusAccepted,
		models.ApplicationStatusDeclined,
		models.ApplicationStatusWithdrawn,
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
	if filter.Status != "" && !filter.Status.IsValid() {
		return nil, nil, fmt.Errorf("%w: status must be applied, tasks_submitted, or an application status", domain.ErrInvalidInput)
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

// Create validates input and creates a mentee application, or a reapplication after a withdrawal.
// Mentors do not apply here: they request to join a program through its members (FR-023).
// Guards enforced:
//   - The term takes applications now (FR-016, ProgramTerm.AcceptsApplications).
//   - The applicant has a mentee profile, created only once the eligibility criteria are
//     confirmed (FR-025, FR-028).
//   - Reapplication only from withdrawn, never from declined, up to the withdrawal limit (FR-030).
//   - Under lock, in the repository: the program is published, and the applicant holds no
//     accepted or graduated mentee application.
func (s *ApplicationService) Create(ctx context.Context, programTermID string, input models.ApplicationCreateInput) (*models.Application, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.Create")
	defer span.End()

	if input.UserID == "" {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	}
	if input.Role == "" {
		input.Role = models.ApplicationRoleMentee
	}
	if input.Role != models.ApplicationRoleMentee {
		return nil, fmt.Errorf("%w: role must be mentee; mentors request to join a program through its members", domain.ErrInvalidInput)
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
	switch term.ApplicationWindow(time.Now()) {
	case models.ApplicationWindowUnset:
		return nil, fmt.Errorf("%w: this term has no application window", domain.ErrIneligible)
	case models.ApplicationWindowUpcoming:
		return nil, fmt.Errorf("%w: application window has not opened yet", domain.ErrIneligible)
	case models.ApplicationWindowClosed:
		return nil, fmt.Errorf("%w: application window has closed", domain.ErrIneligible)
	}

	profiles, err := s.profileRepo.CountActiveMenteeProfiles(ctx, input.UserID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("check mentee profile: %w", err)
	}
	if profiles == 0 {
		return nil, fmt.Errorf("%w: create a mentee profile, confirming the eligibility criteria, before applying", domain.ErrIneligible)
	}

	// Reapply guard: no existing non-terminal application for this term+user.
	existing, err := s.repo.FindByTermAndUser(ctx, programTermID, input.UserID, input.Role)
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
		withdrawn, err := s.repo.CountWithdrawnByTermAndUser(ctx, programTermID, input.UserID, input.Role)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("count withdrawn applications: %w", err)
		}
		if withdrawn >= models.MaxWithdrawnApplicationsPerTerm {
			return nil, fmt.Errorf("%w: reapplication limit reached: you have withdrawn %d applications for this term and cannot apply to it again", domain.ErrIneligible, withdrawn)
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
		s.notifier.NotifyMenteeApplicationReceived(ctx, a.ID)
		awaitCreatorAccess(ctx, s.access, applicationFGAObjectPrefix+a.ID, applicationCreatorRelation)
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
	s.notifier.NotifyMenteeApplicationReceived(ctx, a.ID)
	awaitCreatorAccess(ctx, s.access, applicationFGAObjectPrefix+a.ID, applicationCreatorRelation)
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
	// Program update requires a name; decodePrerequisites skips one saved before it did.
	prerequisites, err := decodePrerequisites(prog.TaskTemplates)
	if err != nil {
		return nil, fmt.Errorf("decode program task templates: %w", err)
	}
	tasks := make([]models.TaskCreateInput, 0, len(prerequisites))
	for _, p := range prerequisites {
		tasks = append(tasks, p.NewTask(uuid.New().String(), programTermID, input.UserID))
	}
	return tasks, nil
}

// Update applies status changes to an application.
// Enforces the state machine defined in applicationTransitions.
// When accepting, attendance_type is required.
// A mentee is notified when their application is accepted, declined, withdrawn, or graduated;
// program admins are notified when a mentor withdraws their application.
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
			// A status change is a program admin decision, as the application's manager relation
			// has it at the gateway; mentors review but do not decide.
			if next != models.ApplicationStatusWithdrawn || input.ActorID != current.UserID {
				if err := s.requireProgramAdmin(ctx, current.ProgramTermID, input.ActorID); err != nil {
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
			// The transition was validated against current.Status, so the write must apply only while the row still has it.
			input.ExpectedStatus = &current.Status

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
	if input.Status != nil && a.Role == models.ApplicationRoleMentee {
		switch *input.Status {
		case models.ApplicationStatusAccepted:
			attType := ""
			if a.AttendanceType != nil {
				attType = string(*a.AttendanceType)
			}
			s.notifier.NotifyMenteeAccepted(ctx, id, attType)
		case models.ApplicationStatusDeclined:
			s.notifier.NotifyMenteeDeclined(ctx, id)
		case models.ApplicationStatusWithdrawn:
			s.notifier.NotifyMenteeWithdrawn(ctx, id)
		case models.ApplicationStatusGraduated:
			s.notifier.NotifyMenteeGraduated(ctx, id)
		}
	}
	if input.Status != nil && a.Role == models.ApplicationRoleMentor {
		withdrew := *input.Status == models.ApplicationStatusWithdrawn
		// A decline answers only an outstanding request (ExpectedStatus is the status the write applied
		// from). Declining an accepted mentor leaves their approved membership in place, so it sends nothing.
		declinedRequest := *input.Status == models.ApplicationStatusDeclined && input.ExpectedStatus != nil &&
			*input.ExpectedStatus == models.ApplicationStatusPending
		if withdrew || declinedRequest {
			// The write has landed, so a failed lookup costs only the notice.
			if term, err := s.termRepo.GetByID(ctx, a.ProgramTermID); err != nil {
				span.RecordError(err)
			} else if withdrew {
				s.notifier.NotifyAdminMentorWithdrew(ctx, term.ProgramID, a.UserID)
			} else {
				s.notifier.NotifyMentorDeclined(ctx, term.ProgramID, a.UserID)
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

// Delete removes an application and its authorization descendants. Only a program admin may delete.
func (s *ApplicationService) Delete(ctx context.Context, id, actorID string) error {
	application, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get application for delete: %w", err)
	}
	if err := s.requireProgramAdmin(ctx, application.ProgramTermID, actorID); err != nil {
		return err
	}
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
	if err := s.requireProgramAdmin(ctx, application.ProgramTermID, actorID); err != nil {
		return nil, err
	}
	return s.assistedWithdraw(ctx, application)
}

// WithdrawForMenteeAfterGatewayAuthorization applies the state transition after
// Heimdall has verified the caller's manager relation on the application.
func (s *ApplicationService) WithdrawForMenteeAfterGatewayAuthorization(ctx context.Context, id string) (*models.Application, error) {
	application, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get application for assisted withdrawal: %w", err)
	}
	return s.assistedWithdraw(ctx, application)
}

// assistedWithdraw withdraws an application on the mentee's behalf, notifying them unless it was already withdrawn.
// A declined or graduated application is a final outcome and is refused: withdrawing it would let the mentee
// reapply to the term, which FR-030 forbids after a decline.
// The write applies only while the row keeps the status read before it, so a concurrent change fails
// rather than being overwritten and notified twice.
func (s *ApplicationService) assistedWithdraw(ctx context.Context, application *models.Application) (*models.Application, error) {
	if application.Status == models.ApplicationStatusDeclined || application.Status == models.ApplicationStatusGraduated {
		return nil, fmt.Errorf("%w: cannot withdraw a %s application", domain.ErrInvalidStateTransition, application.Status)
	}
	status := models.ApplicationStatusWithdrawn
	updated, err := s.repo.Update(ctx, application.ID, models.ApplicationUpdateInput{Status: &status, ExpectedStatus: &application.Status})
	if err != nil {
		return nil, fmt.Errorf("assisted withdrawal: %w", err)
	}
	if application.Status != models.ApplicationStatusWithdrawn && updated.Role == models.ApplicationRoleMentee {
		s.notifier.NotifyMenteeWithdrawn(ctx, updated.ID)
	}
	return updated, nil
}

// BulkDeclineByTerm moves all pending applications in a term to declined and notifies each mentee.
// Only a program admin may bulk-decline.
func (s *ApplicationService) BulkDeclineByTerm(ctx context.Context, termID, actorID string) (int, error) {
	ctx, span := applicationSvcTracer.Start(ctx, "ApplicationService.BulkDeclineByTerm")
	defer span.End()
	span.SetAttributes(attribute.String("term.id", termID))

	if err := s.requireProgramAdmin(ctx, termID, actorID); err != nil {
		return 0, err
	}

	declined, err := s.repo.BulkDeclineByTerm(ctx, termID)
	if err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("bulk decline: %w", err)
	}
	notifyMenteesDeclined(ctx, s.notifier, declined)
	return len(declined), nil
}

// notifyMenteesDeclined tells each mentee among the bulk-declined applications; mentor applications get no email.
func notifyMenteesDeclined(ctx context.Context, notifier domain.Notifier, declined []*models.Application) {
	for _, a := range declined {
		if a.Role == models.ApplicationRoleMentee {
			notifier.NotifyMenteeDeclined(ctx, a.ID)
		}
	}
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
