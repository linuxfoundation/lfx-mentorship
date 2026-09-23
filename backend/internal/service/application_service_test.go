// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package service_test contains unit tests for the application service layer.
package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

// ── stubs ───────────────────────────────────────────────────────────────────

type stubAppRepo struct {
	getByID           func(context.Context, string) (*models.Application, error)
	listByProgramTerm func(context.Context, string, models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	listByProgram     func(context.Context, string, models.ProgramApplicationFilter) ([]*models.ProgramApplicationRow, *models.PaginationMeta, error)
	listByUser        func(context.Context, string, models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	create            func(context.Context, string, models.ApplicationCreateInput) (*models.Application, error)
	reapply           func(context.Context, string, string, models.ApplicationCreateInput) (*models.Application, error)
	reapplyWithTasks  func(context.Context, string, string, models.ApplicationCreateInput, []models.TaskCreateInput) (*models.Application, error)
	update            func(context.Context, string, models.ApplicationUpdateInput) (*models.Application, error)
	delete            func(context.Context, string) error
	countBlocking     func(context.Context, string) (int, error)
	countAccepted     func(context.Context, string) (int, error)
	findByTermAndUser func(context.Context, string, string) (*models.Application, error)
	bulkDecline       func(context.Context, string) (int, error)
	listPastMentees   func(context.Context, string) ([]*models.Application, error)
}

func (m *stubAppRepo) GetByID(ctx context.Context, id string) (*models.Application, error) {
	if m.getByID != nil {
		return m.getByID(ctx, id)
	}
	return &models.Application{ID: id, Status: "pending"}, nil
}
func (m *stubAppRepo) ListByProgramTerm(ctx context.Context, id string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
	if m.listByProgramTerm != nil {
		return m.listByProgramTerm(ctx, id, f)
	}
	return nil, &models.PaginationMeta{}, nil
}
func (m *stubAppRepo) ListByProgram(ctx context.Context, id string, f models.ProgramApplicationFilter) ([]*models.ProgramApplicationRow, *models.PaginationMeta, error) {
	if m.listByProgram != nil {
		return m.listByProgram(ctx, id, f)
	}
	return []*models.ProgramApplicationRow{}, &models.PaginationMeta{}, nil
}
func (m *stubAppRepo) ListByUser(ctx context.Context, id string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
	if m.listByUser != nil {
		return m.listByUser(ctx, id, f)
	}
	return nil, &models.PaginationMeta{}, nil
}
func (m *stubAppRepo) Create(ctx context.Context, termID string, in models.ApplicationCreateInput) (*models.Application, error) {
	if m.create != nil {
		return m.create(ctx, termID, in)
	}
	return &models.Application{Status: in.Status, UserID: in.UserID, Role: in.Role}, nil
}
func (m *stubAppRepo) CreateWithTasks(ctx context.Context, termID string, in models.ApplicationCreateInput, _ []models.TaskCreateInput) (*models.Application, error) {
	return m.Create(ctx, termID, in)
}
func (m *stubAppRepo) Reapply(ctx context.Context, oldID, termID string, in models.ApplicationCreateInput) (*models.Application, error) {
	if m.reapply != nil {
		return m.reapply(ctx, oldID, termID, in)
	}
	return &models.Application{ID: in.ID, ProgramTermID: termID, Status: in.Status, UserID: in.UserID, Role: in.Role}, nil
}
func (m *stubAppRepo) ReapplyWithTasks(ctx context.Context, oldID, termID string, in models.ApplicationCreateInput, tasks []models.TaskCreateInput) (*models.Application, error) {
	if m.reapplyWithTasks != nil {
		return m.reapplyWithTasks(ctx, oldID, termID, in, tasks)
	}
	return m.Reapply(ctx, oldID, termID, in)
}
func (m *stubAppRepo) Update(ctx context.Context, id string, in models.ApplicationUpdateInput) (*models.Application, error) {
	if m.update != nil {
		return m.update(ctx, id, in)
	}
	return &models.Application{ID: id}, nil
}
func (m *stubAppRepo) Delete(ctx context.Context, id string) error {
	if m.delete != nil {
		return m.delete(ctx, id)
	}
	return nil
}
func (m *stubAppRepo) CountBlockingAppsForProgram(ctx context.Context, id string) (int, error) {
	if m.countBlocking != nil {
		return m.countBlocking(ctx, id)
	}
	return 0, nil
}
func (m *stubAppRepo) CountAcceptedByTerm(ctx context.Context, id string) (int, error) {
	if m.countAccepted != nil {
		return m.countAccepted(ctx, id)
	}
	return 0, nil
}
func (m *stubAppRepo) CountByTerm(context.Context, string) (int, error) { return 0, nil }
func (m *stubAppRepo) FindByTermAndUser(ctx context.Context, termID, userID string) (*models.Application, error) {
	if m.findByTermAndUser != nil {
		return m.findByTermAndUser(ctx, termID, userID)
	}
	return nil, nil
}
func (m *stubAppRepo) BulkDeclineByTerm(ctx context.Context, termID string) (int, error) {
	if m.bulkDecline != nil {
		return m.bulkDecline(ctx, termID)
	}
	return 0, nil
}
func (m *stubAppRepo) ListPastMenteesByTerm(ctx context.Context, termID string) ([]*models.Application, error) {
	if m.listPastMentees != nil {
		return m.listPastMentees(ctx, termID)
	}
	return nil, nil
}

type stubTermRepo struct {
	getByID            func(context.Context, string) (*models.ProgramTerm, error)
	getByProgramAndID  func(context.Context, string, string) (*models.ProgramTerm, error)
	listByProgram      func(context.Context, string, models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error)
	create             func(context.Context, models.ProgramTermCreateInput) (*models.ProgramTerm, error)
	update             func(context.Context, string, models.ProgramTermUpdateInput) (*models.ProgramTerm, error)
	delete             func(context.Context, string) error
	countOpenByProgram func(context.Context, string) (int, error)
}

func (m *stubTermRepo) GetByID(ctx context.Context, id string) (*models.ProgramTerm, error) {
	if m.getByID != nil {
		return m.getByID(ctx, id)
	}
	return &models.ProgramTerm{ID: id, Status: "open"}, nil
}
func (m *stubTermRepo) GetByProgramAndID(ctx context.Context, programID, id string) (*models.ProgramTerm, error) {
	if m.getByProgramAndID != nil {
		return m.getByProgramAndID(ctx, programID, id)
	}
	return &models.ProgramTerm{ID: id, ProgramID: programID, Status: "open"}, nil
}
func (m *stubTermRepo) ListByProgram(ctx context.Context, id string, f models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error) {
	if m.listByProgram != nil {
		return m.listByProgram(ctx, id, f)
	}
	return nil, &models.PaginationMeta{}, nil
}
func (m *stubTermRepo) ListManagementByProgram(context.Context, string, models.ProgramTermFilter) ([]*models.ProgramTermManagementRow, *models.PaginationMeta, error) {
	return []*models.ProgramTermManagementRow{}, &models.PaginationMeta{}, nil
}
func (m *stubTermRepo) Create(ctx context.Context, in models.ProgramTermCreateInput) (*models.ProgramTerm, error) {
	if m.create != nil {
		return m.create(ctx, in)
	}
	return &models.ProgramTerm{}, nil
}
func (m *stubTermRepo) Update(ctx context.Context, id string, in models.ProgramTermUpdateInput) (*models.ProgramTerm, error) {
	if m.update != nil {
		return m.update(ctx, id, in)
	}
	return &models.ProgramTerm{ID: id}, nil
}
func (m *stubTermRepo) Delete(ctx context.Context, id string) error {
	if m.delete != nil {
		return m.delete(ctx, id)
	}
	return nil
}
func (m *stubTermRepo) CountOpenTermsByProgram(ctx context.Context, id string) (int, error) {
	if m.countOpenByProgram != nil {
		return m.countOpenByProgram(ctx, id)
	}
	return 1, nil
}

type stubProgRepo struct {
	getByID           func(context.Context, string) (*models.Program, error)
	getBySlug         func(context.Context, string) (*models.Program, error)
	list              func(context.Context, models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error)
	managementSummary func(context.Context, string) (*models.ProgramManagementSummary, error)
	nameAvailable     func(context.Context, string, string) (bool, error)
	listCatalog       func(context.Context, models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error)
	getCatalog        func(context.Context, string) (*models.ProgramCatalogItem, error)
	listMentees       func(context.Context, string) ([]*models.ProgramCatalogMentee, error)
	create            func(context.Context, models.ProgramCreateInput) (*models.Program, error)
	update            func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error)
	delete            func(context.Context, string) error
	listSkills        func(context.Context, string) ([]*models.ProgramSkill, error)
	addSkill          func(context.Context, string, models.ProgramSkillCreateInput) (*models.ProgramSkill, error)
	deleteSkill       func(context.Context, string, string) error
	getFundingStats   func(context.Context, string) (*models.ProgramFundingStats, error)
}

func (m *stubProgRepo) GetByID(ctx context.Context, id string) (*models.Program, error) {
	if m.getByID != nil {
		return m.getByID(ctx, id)
	}
	return &models.Program{ID: id, Status: models.ProgramStatusPublished}, nil
}
func (m *stubProgRepo) GetBySlug(ctx context.Context, slug string) (*models.Program, error) {
	if m.getBySlug != nil {
		return m.getBySlug(ctx, slug)
	}
	return &models.Program{}, nil
}
func (m *stubProgRepo) List(ctx context.Context, f models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error) {
	if m.list != nil {
		return m.list(ctx, f)
	}
	return nil, &models.PaginationMeta{}, nil
}
func (m *stubProgRepo) ListManaged(ctx context.Context, userID string, f models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error) {
	return []*models.Program{}, &models.PaginationMeta{}, nil
}
func (m *stubProgRepo) GetEnrollmentTemplate(ctx context.Context, userID, programID string) (*models.ProgramEnrollmentTemplate, error) {
	return &models.ProgramEnrollmentTemplate{}, nil
}
func (m *stubProgRepo) GetManagementSummary(ctx context.Context, id string) (*models.ProgramManagementSummary, error) {
	if m.managementSummary != nil {
		return m.managementSummary(ctx, id)
	}
	return &models.ProgramManagementSummary{}, nil
}
func (m *stubProgRepo) GetHeaderProjection(ctx context.Context, id string) (*models.ProgramHeaderProjection, error) {
	return &models.ProgramHeaderProjection{Program: &models.Program{ID: id}}, nil
}
func (m *stubProgRepo) NameAvailable(ctx context.Context, name, excludeProgramID string) (bool, error) {
	if m.nameAvailable != nil {
		return m.nameAvailable(ctx, name, excludeProgramID)
	}
	return true, nil
}
func (m *stubProgRepo) ListCatalog(ctx context.Context, f models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error) {
	if m.listCatalog != nil {
		return m.listCatalog(ctx, f)
	}
	return []*models.ProgramCatalogItem{}, &models.PaginationMeta{}, nil
}
func (m *stubProgRepo) GetCatalog(ctx context.Context, id string) (*models.ProgramCatalogItem, error) {
	if m.getCatalog != nil {
		return m.getCatalog(ctx, id)
	}
	return &models.ProgramCatalogItem{Program: models.Program{ID: id, Status: models.ProgramStatusPublished}}, nil
}
func (m *stubProgRepo) ListCatalogMentees(ctx context.Context, id string) ([]*models.ProgramCatalogMentee, error) {
	if m.listMentees != nil {
		return m.listMentees(ctx, id)
	}
	return []*models.ProgramCatalogMentee{}, nil
}
func (m *stubProgRepo) Create(ctx context.Context, in models.ProgramCreateInput) (*models.Program, error) {
	if m.create != nil {
		return m.create(ctx, in)
	}
	return &models.Program{}, nil
}
func (m *stubProgRepo) CreateEnrollment(ctx context.Context, in models.ProgramEnrollmentInput) (*models.Program, error) {
	return m.Create(ctx, in.Program)
}
func (m *stubProgRepo) Update(ctx context.Context, id string, in models.ProgramUpdateInput) (*models.Program, error) {
	if m.update != nil {
		return m.update(ctx, id, in)
	}
	return &models.Program{ID: id}, nil
}
func (m *stubProgRepo) Delete(ctx context.Context, id string) error {
	if m.delete != nil {
		return m.delete(ctx, id)
	}
	return nil
}
func (m *stubProgRepo) ListSkills(ctx context.Context, id string) ([]*models.ProgramSkill, error) {
	if m.listSkills != nil {
		return m.listSkills(ctx, id)
	}
	return nil, nil
}
func (m *stubProgRepo) AddSkill(ctx context.Context, id string, in models.ProgramSkillCreateInput) (*models.ProgramSkill, error) {
	if m.addSkill != nil {
		return m.addSkill(ctx, id, in)
	}
	return &models.ProgramSkill{}, nil
}
func (m *stubProgRepo) DeleteSkill(ctx context.Context, programID, id string) error {
	if m.deleteSkill != nil {
		return m.deleteSkill(ctx, programID, id)
	}
	return nil
}
func (m *stubProgRepo) GetFundingStats(ctx context.Context, id string) (*models.ProgramFundingStats, error) {
	if m.getFundingStats != nil {
		return m.getFundingStats(ctx, id)
	}
	return &models.ProgramFundingStats{}, nil
}

type stubTaskRepo struct {
	getByID                         func(context.Context, string) (*models.Task, error)
	create                          func(context.Context, string, models.TaskCreateInput) (*models.Task, error)
	update                          func(context.Context, string, models.TaskUpdateInput) (*models.Task, error)
	delete                          func(context.Context, string) error
	listByApplication               func(context.Context, string, models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	listByProgramTerm               func(context.Context, string, models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	countPrerequisitesByApplication func(context.Context, string) (int, int, error)
}

func (m *stubTaskRepo) GetByID(ctx context.Context, id string) (*models.Task, error) {
	if m.getByID != nil {
		return m.getByID(ctx, id)
	}
	return &models.Task{ID: id}, nil
}
func (m *stubTaskRepo) Create(ctx context.Context, appID string, in models.TaskCreateInput) (*models.Task, error) {
	if m.create != nil {
		return m.create(ctx, appID, in)
	}
	return &models.Task{}, nil
}
func (m *stubTaskRepo) Update(ctx context.Context, id string, in models.TaskUpdateInput) (*models.Task, error) {
	if m.update != nil {
		return m.update(ctx, id, in)
	}
	return &models.Task{ID: id}, nil
}
func (m *stubTaskRepo) Delete(ctx context.Context, id string) error {
	if m.delete != nil {
		return m.delete(ctx, id)
	}
	return nil
}
func (m *stubTaskRepo) ListByApplication(ctx context.Context, appID string, f models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	if m.listByApplication != nil {
		return m.listByApplication(ctx, appID, f)
	}
	return nil, &models.PaginationMeta{}, nil
}
func (m *stubTaskRepo) ListByProgramTerm(ctx context.Context, termID string, f models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	if m.listByProgramTerm != nil {
		return m.listByProgramTerm(ctx, termID, f)
	}
	return nil, &models.PaginationMeta{}, nil
}
func (m *stubTaskRepo) CountPrerequisiteTasksByApplication(ctx context.Context, appID string) (int, int, error) {
	if m.countPrerequisitesByApplication != nil {
		return m.countPrerequisitesByApplication(ctx, appID)
	}
	return 0, 0, nil
}

type stubNotifier struct {
	mentorInvitedCalls  int
	mentorDeclinedCalls int
	tasksSubmittedCalls int
	menteeAcceptedCalls int
}

func (n *stubNotifier) NotifyMentorInvited(_ context.Context, _, _, _ string) { n.mentorInvitedCalls++ }
func (n *stubNotifier) NotifyMentorDeclined(_ context.Context, _, _ string)   { n.mentorDeclinedCalls++ }
func (n *stubNotifier) NotifyAdminTasksSubmitted(_ context.Context, _ string) {
	n.tasksSubmittedCalls++
}
func (n *stubNotifier) NotifyMenteeAccepted(_ context.Context, _, _ string) { n.menteeAcceptedCalls++ }

// ── helpers ─────────────────────────────────────────────────────────────────

func newApplicationSvc(appRepo *stubAppRepo, taskRepo *stubTaskRepo, termRepo *stubTermRepo, progRepo *stubProgRepo) *service.ApplicationService {
	return service.NewApplicationService(appRepo, taskRepo, termRepo, progRepo, &stubMemberRepo{}, &stubNotifier{})
}

func newApplicationSvcWithMember(appRepo *stubAppRepo, taskRepo *stubTaskRepo, termRepo *stubTermRepo, progRepo *stubProgRepo, memberRepo *stubMemberRepo) *service.ApplicationService {
	return service.NewApplicationService(appRepo, taskRepo, termRepo, progRepo, memberRepo, &stubNotifier{})
}

func openTerm(t time.Time) *models.ProgramTerm {
	past := t.Add(-24 * time.Hour)
	future := t.Add(24 * time.Hour)
	return &models.ProgramTerm{
		ID:                   "term-1",
		Status:               "open",
		ApplicationStartDate: &past,
		ApplicationEndDate:   &future,
	}
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestApplicationService_Create_ForcesStatusPending(t *testing.T) {
	var capturedStatus models.ApplicationStatus
	repo := &stubAppRepo{
		create: func(_ context.Context, _ string, in models.ApplicationCreateInput) (*models.Application, error) {
			capturedStatus = in.Status
			return &models.Application{Status: in.Status}, nil
		},
	}
	termRepo := &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return openTerm(time.Now()), nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	_, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{
		UserID: "u1",
		Role:   "mentee",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if capturedStatus != "pending" {
		t.Errorf("status = %q; want %q", capturedStatus, "pending")
	}
}

func TestApplicationService_Create_TermNotOpen(t *testing.T) {
	termRepo := &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: "term-1", Status: "closed"}, nil
		},
	}
	svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	_, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
	if !errors.Is(err, domain.ErrIneligible) {
		t.Errorf("expected ErrIneligible, got %v", err)
	}
}

func TestApplicationService_Create_WindowNotYetOpen(t *testing.T) {
	future := time.Now().Add(48 * time.Hour)
	termRepo := &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: "t1", Status: "open", ApplicationStartDate: &future}, nil
		},
	}
	svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
	if !errors.Is(err, domain.ErrIneligible) {
		t.Errorf("expected ErrIneligible, got %v", err)
	}
}

func TestApplicationService_Create_WindowClosed(t *testing.T) {
	past := time.Now().Add(-48 * time.Hour)
	termRepo := &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: "t1", Status: "open", ApplicationEndDate: &past}, nil
		},
	}
	svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
	if !errors.Is(err, domain.ErrIneligible) {
		t.Errorf("expected ErrIneligible, got %v", err)
	}
}

func TestApplicationService_Create_DuplicateActive_Blocked(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
		return openTerm(time.Now()), nil
	}}
	repo := &stubAppRepo{
		findByTermAndUser: func(_ context.Context, _, _ string) (*models.Application, error) {
			return &models.Application{ID: "existing", Status: "accepted"}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestApplicationService_Create_DeclinedReapply_Blocked(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
		return openTerm(time.Now()), nil
	}}
	repo := &stubAppRepo{
		findByTermAndUser: func(_ context.Context, _, _ string) (*models.Application, error) {
			return &models.Application{ID: "existing", Status: "declined"}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestApplicationService_Create_WithdrawnReapply_ReplacesAtomically(t *testing.T) {
	var replaced string
	termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
		return openTerm(time.Now()), nil
	}}
	repo := &stubAppRepo{
		findByTermAndUser: func(_ context.Context, _, _ string) (*models.Application, error) {
			return &models.Application{ID: "old", Status: "withdrawn"}, nil
		},
		reapply: func(_ context.Context, oldID, _ string, _ models.ApplicationCreateInput) (*models.Application, error) {
			replaced = oldID
			return &models.Application{ID: "new", Status: "pending"}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if replaced != "old" {
		t.Errorf("old withdrawn application was not atomically replaced; got oldID=%q", replaced)
	}
}

func TestApplicationService_Create_WithdrawnReapply_ClonesPrerequisiteTasks(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
		term := openTerm(time.Now())
		term.ProgramID = "program-1"
		return term, nil
	}}
	repo := &stubAppRepo{
		findByTermAndUser: func(_ context.Context, _, _ string) (*models.Application, error) {
			return &models.Application{ID: "old", Status: models.ApplicationStatusWithdrawn}, nil
		},
		reapply: func(_ context.Context, oldID, termID string, in models.ApplicationCreateInput) (*models.Application, error) {
			if oldID != "old" || termID != "term-1" {
				t.Fatalf("reapply arguments = oldID %q, termID %q", oldID, termID)
			}
			return &models.Application{ID: in.ID, ProgramTermID: termID, UserID: in.UserID, Status: in.Status, Role: in.Role}, nil
		},
	}
	taskCount := 0
	repo.reapplyWithTasks = func(_ context.Context, _ string, _ string, _ models.ApplicationCreateInput, tasks []models.TaskCreateInput) (*models.Application, error) {
		taskCount = len(tasks)
		if len(tasks) != 1 || tasks[0].Category == nil || *tasks[0].Category != models.TaskCategoryPrerequisite || tasks[0].Status != models.TaskStatusIncomplete {
			t.Fatalf("task payload = %+v; want one incomplete prerequisite", tasks)
		}
		return &models.Application{ID: "new", ProgramTermID: "term-1", UserID: "u1", Status: models.ApplicationStatusPending, Role: models.ApplicationRoleMentee}, nil
	}
	progRepo := &stubProgRepo{getByID: func(_ context.Context, _ string) (*models.Program, error) {
		return &models.Program{TaskTemplates: json.RawMessage(`[{"name":"Code of Conduct","description":"Read it"}]`)}, nil
	}}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, progRepo)
	_, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{
		UserID: "u1",
		Role:   models.ApplicationRoleMentee,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if taskCount != 1 {
		t.Fatalf("cloned task count = %d; want 1", taskCount)
	}
}

func TestApplicationService_Update_ValidTransition(t *testing.T) {
	repo := &stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, Status: "pending"}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
	next := models.ApplicationStatusAccepted
	attType := models.AttendanceTypeFullTime
	_, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next, AttendanceType: &attType})
	if err != nil {
		t.Errorf("expected valid transition pending→accepted, got %v", err)
	}
}

func TestApplicationService_Update_AcceptedMenteeSendsNotification(t *testing.T) {
	notifier := &stubNotifier{}
	repo := &stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, Status: models.ApplicationStatusPending, ProgramTermID: "term-1", UserID: "mentee-1", Role: models.ApplicationRoleMentee}, nil
		},
		update: func(_ context.Context, id string, _ models.ApplicationUpdateInput) (*models.Application, error) {
			attType := models.AttendanceTypeFullTime
			return &models.Application{ID: id, Status: models.ApplicationStatusAccepted, ProgramTermID: "term-1", UserID: "mentee-1", Role: models.ApplicationRoleMentee, AttendanceType: &attType}, nil
		},
	}
	svc := service.NewApplicationService(repo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{}, &stubMemberRepo{}, notifier)
	next := models.ApplicationStatusAccepted
	attType := models.AttendanceTypeFullTime
	if _, err := svc.Update(context.Background(), "app-mentee-1", models.ApplicationUpdateInput{Status: &next, AttendanceType: &attType}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if notifier.menteeAcceptedCalls != 1 {
		t.Fatalf("mentee notifications = %d; want 1", notifier.menteeAcceptedCalls)
	}
}

// An enrolled mentee stays "accepted" for the whole term and graduates
// directly from it — there is no intermediate "active" status.
func TestApplicationService_Update_AcceptedToGraduated(t *testing.T) {
	repo := &stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, Status: models.ApplicationStatusAccepted}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
	next := models.ApplicationStatusGraduated
	if _, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next}); err != nil {
		t.Errorf("expected valid transition accepted→graduated, got %v", err)
	}
}

func TestApplicationService_Update_InvalidTransition(t *testing.T) {
	repo := &stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, Status: "pending"}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
	next := models.ApplicationStatusGraduated
	_, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition, got %v", err)
	}
}

func TestApplicationService_Update_WithdrawnTerminal(t *testing.T) {
	repo := &stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, Status: "withdrawn"}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
	next := models.ApplicationStatusPending
	_, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next})
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition for terminal withdrawn, got %v", err)
	}
}

func TestApplicationService_Update_InvalidProgramTermStatus_Rejected(t *testing.T) {
	svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
	bad := models.ProgramTermStatus("ajar") // not a member of the enum
	_, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{ProgramTermStatus: &bad})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for unknown program_term_status, got %v", err)
	}
}

func TestApplicationService_Create_InvalidAttendanceType_Rejected(t *testing.T) {
	svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
	bad := models.AttendanceType("weekends") // not a member of the enum
	_, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{
		UserID:         "u1",
		Role:           models.ApplicationRoleMentee,
		AttendanceType: &bad,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for unknown attendance_type, got %v", err)
	}
}

func TestApplicationService_Create_InvalidProgramTermStatus_Rejected(t *testing.T) {
	svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
	bad := models.ProgramTermStatus("ajar") // not a member of the enum
	_, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{
		UserID:            "u1",
		Role:              models.ApplicationRoleMentee,
		ProgramTermStatus: &bad,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for unknown program_term_status, got %v", err)
	}
}

func TestApplicationService_GetByIDForActor_ApplicantAllowed(t *testing.T) {
	svc := newApplicationSvc(&stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, UserID: "user-1", ProgramTermID: "term-1"}, nil
		},
	}, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})

	app, err := svc.GetByIDForActor(context.Background(), "app-1", "user-1")
	if err != nil {
		t.Fatalf("GetByIDForActor: %v", err)
	}
	if app.ID != "app-1" {
		t.Fatalf("app.ID = %q; want app-1", app.ID)
	}
}

func TestApplicationService_GetByIDForActor_ActiveReviewerAllowed(t *testing.T) {
	active := models.ProgramMemberStatusActive
	svc := newApplicationSvcWithMember(&stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, UserID: "user-1", ProgramTermID: "term-1"}, nil
		},
	}, &stubTaskRepo{}, &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: "term-1", ProgramID: "prog-1"}, nil
		},
	}, &stubProgRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: models.MemberTypeMentor, Status: &active}, nil
		},
	})

	if _, err := svc.GetByIDForActor(context.Background(), "app-1", "mentor-1"); err != nil {
		t.Fatalf("GetByIDForActor: %v", err)
	}
}

func TestApplicationService_GetByIDForActor_NonMemberForbidden(t *testing.T) {
	svc := newApplicationSvcWithMember(&stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, UserID: "user-1", ProgramTermID: "term-1"}, nil
		},
	}, &stubTaskRepo{}, &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: "term-1", ProgramID: "prog-1"}, nil
		},
	}, &stubProgRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return nil, domain.ErrProgramMemberNotFound
		},
	})

	_, err := svc.GetByIDForActor(context.Background(), "app-1", "stranger-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestApplicationService_GetByIDForActor_InactiveOrWrongRoleForbidden(t *testing.T) {
	inactive := models.ProgramMemberStatusInvited
	svc := newApplicationSvcWithMember(&stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, UserID: "user-1", ProgramTermID: "term-1"}, nil
		},
	}, &stubTaskRepo{}, &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: "term-1", ProgramID: "prog-1"}, nil
		},
	}, &stubProgRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: models.MemberTypeProgramAdmin, Status: &inactive}, nil
		},
	})

	_, err := svc.GetByIDForActor(context.Background(), "app-1", "admin-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestApplicationService_GetByIDForActor_MembershipLookupFailurePropagates(t *testing.T) {
	opErr := errors.New("membership lookup failed")
	svc := newApplicationSvcWithMember(&stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, UserID: "user-1", ProgramTermID: "term-1"}, nil
		},
	}, &stubTaskRepo{}, &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: "term-1", ProgramID: "prog-1"}, nil
		},
	}, &stubProgRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return nil, opErr
		},
	})

	_, err := svc.GetByIDForActor(context.Background(), "app-1", "mentor-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, opErr) {
		t.Fatalf("expected wrapped lookup error, got %v", err)
	}
}

func TestApplicationService_ListByProgramTermForActor_NonReviewerForcesUserFilter(t *testing.T) {
	svc := newApplicationSvcWithMember(&stubAppRepo{
		listByProgramTerm: func(_ context.Context, _ string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
			if f.UserID != "user-2" {
				t.Fatalf("filter.UserID = %q; want user-2", f.UserID)
			}
			return []*models.Application{}, &models.PaginationMeta{}, nil
		},
	}, &stubTaskRepo{}, &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: "term-1", ProgramID: "prog-1"}, nil
		},
	}, &stubProgRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return nil, domain.ErrProgramMemberNotFound
		},
	})

	_, _, err := svc.ListByProgramTermForActor(context.Background(), "term-1", models.ApplicationFilter{}, "user-2")
	if err != nil {
		t.Fatalf("ListByProgramTermForActor: %v", err)
	}
}

func TestApplicationService_ListByProgramTermForActor_ReviewerKeepsFilterUserID(t *testing.T) {
	active := models.ProgramMemberStatusActive
	svc := newApplicationSvcWithMember(&stubAppRepo{
		listByProgramTerm: func(_ context.Context, _ string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
			if f.UserID != "explicit-user" {
				t.Fatalf("filter.UserID = %q; want explicit-user", f.UserID)
			}
			return []*models.Application{}, &models.PaginationMeta{}, nil
		},
	}, &stubTaskRepo{}, &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: "term-1", ProgramID: "prog-1"}, nil
		},
	}, &stubProgRepo{}, &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: models.MemberTypeProgramAdmin, Status: &active}, nil
		},
	})

	_, _, err := svc.ListByProgramTermForActor(context.Background(), "term-1", models.ApplicationFilter{UserID: "explicit-user"}, "admin-1")
	if err != nil {
		t.Fatalf("ListByProgramTermForActor: %v", err)
	}
}
