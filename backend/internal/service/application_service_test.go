// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package service_test contains unit tests for the application service layer.
package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	markTasksSubmit   func(context.Context, string) (bool, error)
	delete            func(context.Context, string) error
	countBlocking     func(context.Context, string) (int, error)
	countAccepted     func(context.Context, string) (int, error)
	findByTermAndUser func(context.Context, string, string, models.ApplicationRole) (*models.Application, error)
	countWithdrawn    func(context.Context, string, string, models.ApplicationRole) (int, error)
	bulkDecline       func(context.Context, string) ([]*models.Application, error)
	listPastMentees   func(context.Context, string) ([]*models.Application, error)
	listRemindersDue  func(context.Context, time.Time) ([]*models.PrerequisiteReminderCandidate, error)
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
func (m *stubAppRepo) MarkTasksSubmitted(ctx context.Context, id string) (bool, error) {
	if m.markTasksSubmit != nil {
		return m.markTasksSubmit(ctx, id)
	}
	return false, nil
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
func (m *stubAppRepo) FindByTermAndUser(ctx context.Context, termID, userID string, role models.ApplicationRole) (*models.Application, error) {
	if m.findByTermAndUser != nil {
		return m.findByTermAndUser(ctx, termID, userID, role)
	}
	return nil, nil
}
func (m *stubAppRepo) CountWithdrawnByTermAndUser(ctx context.Context, termID, userID string, role models.ApplicationRole) (int, error) {
	if m.countWithdrawn != nil {
		return m.countWithdrawn(ctx, termID, userID, role)
	}
	return 1, nil
}
func (m *stubAppRepo) BulkDeclineByTerm(ctx context.Context, termID string) ([]*models.Application, error) {
	if m.bulkDecline != nil {
		return m.bulkDecline(ctx, termID)
	}
	return nil, nil
}
func (m *stubAppRepo) ListPastMenteesByTerm(ctx context.Context, termID string) ([]*models.Application, error) {
	if m.listPastMentees != nil {
		return m.listPastMentees(ctx, termID)
	}
	return nil, nil
}
func (m *stubAppRepo) ListPrerequisiteRemindersDue(ctx context.Context, now time.Time) ([]*models.PrerequisiteReminderCandidate, error) {
	if m.listRemindersDue != nil {
		return m.listRemindersDue(ctx, now)
	}
	return nil, nil
}

type stubTermRepo struct {
	getByID              func(context.Context, string) (*models.ProgramTerm, error)
	getByProgramAndID    func(context.Context, string, string) (*models.ProgramTerm, error)
	listByProgram        func(context.Context, string, models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error)
	create               func(context.Context, models.ProgramTermCreateInput) (*models.ProgramTerm, error)
	update               func(context.Context, string, models.ProgramTermUpdateInput) (*models.ProgramTerm, error)
	delete               func(context.Context, string) error
	countOpenByProgram   func(context.Context, string) (int, error)
	closeWithBulkDecline func(context.Context, string) (*models.ProgramTerm, []*models.Application, error)
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
func (m *stubTermRepo) CloseWithBulkDecline(ctx context.Context, id string) (*models.ProgramTerm, []*models.Application, error) {
	if m.closeWithBulkDecline != nil {
		return m.closeWithBulkDecline(ctx, id)
	}
	return &models.ProgramTerm{}, nil, nil
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
	listAdministered  func(context.Context, string, models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error)
	create            func(context.Context, models.ProgramCreateInput) (*models.Program, error)
	createEnrollment  func(context.Context, models.ProgramEnrollmentInput) (*models.Program, error)
	update            func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error)
	// updateWithPrerequisites, when set, replaces update for UpdateWithPrerequisites.
	updateWithPrerequisites func(context.Context, string, models.ProgramUpdateInput, models.PrerequisiteChanges) (*models.Program, []string, error)
	delete                  func(context.Context, string) error
	listSkills              func(context.Context, string) ([]*models.ProgramSkill, error)
	addSkill                func(context.Context, string, models.ProgramSkillCreateInput) (*models.ProgramSkill, error)
	deleteSkill             func(context.Context, string, string) error
	getFundingStats         func(context.Context, string) (*models.ProgramFundingStats, error)
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
func (m *stubProgRepo) GetEnrollmentTemplate(ctx context.Context, programID string) (*models.ProgramEnrollmentTemplate, error) {
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
func (m *stubProgRepo) ListAdministeredByUser(ctx context.Context, userID string, f models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error) {
	if m.listAdministered != nil {
		return m.listAdministered(ctx, userID, f)
	}
	return []*models.AdministeredProgram{}, &models.PaginationMeta{}, nil
}
func (m *stubProgRepo) Create(ctx context.Context, in models.ProgramCreateInput) (*models.Program, error) {
	if m.create != nil {
		return m.create(ctx, in)
	}
	return &models.Program{}, nil
}
func (m *stubProgRepo) CreateEnrollment(ctx context.Context, in models.ProgramEnrollmentInput) (*models.Program, error) {
	if m.createEnrollment != nil {
		return m.createEnrollment(ctx, in)
	}
	return m.Create(ctx, in.Program)
}
func (m *stubProgRepo) Update(ctx context.Context, id string, in models.ProgramUpdateInput) (*models.Program, error) {
	if m.update != nil {
		return m.update(ctx, id, in)
	}
	return &models.Program{ID: id}, nil
}
func (m *stubProgRepo) UpdateWithPrerequisites(ctx context.Context, id string, in models.ProgramUpdateInput, changes models.PrerequisiteChanges) (*models.Program, []string, error) {
	if m.updateWithPrerequisites != nil {
		return m.updateWithPrerequisites(ctx, id, in, changes)
	}
	p, err := m.Update(ctx, id, in)
	return p, nil, err
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
	getByID           func(context.Context, string) (*models.Task, error)
	create            func(context.Context, string, models.TaskCreateInput) (*models.Task, error)
	update            func(context.Context, string, models.TaskUpdateInput) (*models.Task, error)
	delete            func(context.Context, string) error
	listByApplication func(context.Context, string, models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	listByProgramTerm func(context.Context, string, models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
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

type stubNotifier struct {
	programSubmittedCalls     int
	programSubmitters         []string
	programDecisions          []models.ProgramStatus
	programEditors            []string
	profilesCreated           []models.UserProfileType
	mentorInvitedCalls        int
	mentorInviters            []string
	mentorDeclinedCalls       int
	adminMentorDeclinedCalls  int
	adminMentorAcceptedCalls  int
	adminMentorRequestedCalls int
	adminMentorWithdrewCalls  int
	tasksSubmittedCalls       int
	applicationReceivedCalls  int
	menteeAcceptedCalls       int
	menteeDeclinedCalls       int
	menteeWithdrawnCalls      int
	menteeGraduatedCalls      int
	taskAssignedCalls         int
	prerequisiteReminders     []string
}

func (n *stubNotifier) NotifyProgramSubmitted(_ context.Context, _, submitterID string) {
	n.programSubmittedCalls++
	n.programSubmitters = append(n.programSubmitters, submitterID)
}
func (n *stubNotifier) NotifyProgramDecided(_ context.Context, _ string, status models.ProgramStatus) {
	n.programDecisions = append(n.programDecisions, status)
}
func (n *stubNotifier) NotifyProgramUpdated(_ context.Context, _, editorID string) {
	n.programEditors = append(n.programEditors, editorID)
}
func (n *stubNotifier) NotifyProfileCreated(_ context.Context, _ string, profileType models.UserProfileType) {
	n.profilesCreated = append(n.profilesCreated, profileType)
}
func (n *stubNotifier) NotifyMentorInvited(_ context.Context, _, _, inviterID, _ string) {
	n.mentorInvitedCalls++
	n.mentorInviters = append(n.mentorInviters, inviterID)
}
func (n *stubNotifier) NotifyMentorDeclined(_ context.Context, _, _ string) { n.mentorDeclinedCalls++ }
func (n *stubNotifier) NotifyAdminMentorDeclined(_ context.Context, _, _ string) {
	n.adminMentorDeclinedCalls++
}
func (n *stubNotifier) NotifyAdminMentorAccepted(_ context.Context, _, _ string) {
	n.adminMentorAcceptedCalls++
}
func (n *stubNotifier) NotifyAdminTasksSubmitted(_ context.Context, _ string) {
	n.tasksSubmittedCalls++
}
func (n *stubNotifier) NotifyAdminMentorRequested(_ context.Context, _, _ string) {
	n.adminMentorRequestedCalls++
}
func (n *stubNotifier) NotifyAdminMentorWithdrew(_ context.Context, _, _ string) {
	n.adminMentorWithdrewCalls++
}
func (n *stubNotifier) NotifyMenteeApplicationReceived(_ context.Context, _ string) {
	n.applicationReceivedCalls++
}
func (n *stubNotifier) NotifyMenteeAccepted(_ context.Context, _, _ string) { n.menteeAcceptedCalls++ }
func (n *stubNotifier) NotifyMenteeDeclined(_ context.Context, _ string)    { n.menteeDeclinedCalls++ }
func (n *stubNotifier) NotifyMenteeWithdrawn(_ context.Context, _ string)   { n.menteeWithdrawnCalls++ }
func (n *stubNotifier) NotifyMenteeGraduated(_ context.Context, _ string)   { n.menteeGraduatedCalls++ }
func (n *stubNotifier) NotifyMenteeTaskAssigned(_ context.Context, _ string) {
	n.taskAssignedCalls++
}
func (n *stubNotifier) NotifyMenteePrerequisiteReminder(_ context.Context, applicationID string) {
	n.prerequisiteReminders = append(n.prerequisiteReminders, applicationID)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func newApplicationSvc(appRepo *stubAppRepo, taskRepo *stubTaskRepo, termRepo *stubTermRepo, progRepo *stubProgRepo) *service.ApplicationService {
	return service.NewApplicationService(appRepo, taskRepo, termRepo, progRepo, approvedProgramAdmin(), menteeProfiles(1), &stubNotifier{})
}

func newApplicationSvcWithMember(appRepo *stubAppRepo, taskRepo *stubTaskRepo, termRepo *stubTermRepo, progRepo *stubProgRepo, memberRepo *stubMemberRepo) *service.ApplicationService {
	return service.NewApplicationService(appRepo, taskRepo, termRepo, progRepo, memberRepo, menteeProfiles(1), &stubNotifier{})
}

// approvedProgramAdmin is a member repository in which every actor is an approved program admin,
// which also makes them an active reviewer.
func approvedProgramAdmin() *stubMemberRepo {
	status := models.ProgramMemberStatusApproved
	admin := func(context.Context, string, string) (*models.ProgramMember, error) {
		return &models.ProgramMember{MemberType: models.MemberTypeProgramAdmin, Status: &status}, nil
	}
	return &stubMemberRepo{findActiveReviewer: admin, findActiveAdmin: admin}
}

// approvedMentor is a member repository in which every actor is an approved mentor: an active
// reviewer, but not a program admin.
func approvedMentor() *stubMemberRepo {
	status := models.ProgramMemberStatusApproved
	return &stubMemberRepo{
		findActiveReviewer: func(context.Context, string, string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: models.MemberTypeMentor, Status: &status}, nil
		},
		findActiveAdmin: func(context.Context, string, string) (*models.ProgramMember, error) {
			return nil, domain.ErrProgramMemberNotFound
		},
	}
}

// menteeProfiles is a profile repository reporting count mentee profiles for every user.
func menteeProfiles(count int) *stubUserProfileRepo {
	return &stubUserProfileRepo{countActiveMenteeProfiles: func(context.Context, string) (int, error) { return count, nil }}
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

func TestApplicationService_Create_WindowRefusals(t *testing.T) {
	past, future := time.Now().Add(-48*time.Hour), time.Now().Add(48*time.Hour)
	for name, tc := range map[string]struct {
		term *models.ProgramTerm
		want string
	}{
		"not yet open": {&models.ProgramTerm{ID: "t1", Status: "open", ApplicationStartDate: &future, ApplicationEndDate: &future}, "has not opened yet"},
		"closed":       {&models.ProgramTerm{ID: "t1", Status: "open", ApplicationStartDate: &past, ApplicationEndDate: &past}, "has closed"},
		// As in legacy, a term missing either window date takes no applications.
		"no start date": {&models.ProgramTerm{ID: "t1", Status: "open", ApplicationEndDate: &future}, "no application window"},
		"no end date":   {&models.ProgramTerm{ID: "t1", Status: "open", ApplicationStartDate: &past}, "no application window"},
	} {
		t.Run(name, func(t *testing.T) {
			termRepo := &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) { return tc.term, nil }}
			svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, termRepo, &stubProgRepo{})
			_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
			if !errors.Is(err, domain.ErrIneligible) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v; want ErrIneligible naming %q", err, tc.want)
			}
		})
	}
}

func TestApplicationService_Create_RequiresMenteeProfile(t *testing.T) {
	repo := &stubAppRepo{create: func(context.Context, string, models.ApplicationCreateInput) (*models.Application, error) {
		t.Fatal("an application was created without a mentee profile")
		return nil, nil
	}}
	termRepo := &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) { return openTerm(time.Now()), nil }}
	svc := service.NewApplicationService(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{}, approvedProgramAdmin(), menteeProfiles(0), &stubNotifier{})
	_, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{UserID: "u1", Role: models.ApplicationRoleMentee})
	if !errors.Is(err, domain.ErrIneligible) || !strings.Contains(err.Error(), "mentee profile") {
		t.Fatalf("err = %v; want ErrIneligible naming the mentee profile", err)
	}
}

// Mentors join a program through a membership request, not through the apply route.
func TestApplicationService_Create_MenteeOnly(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) { return openTerm(time.Now()), nil }}
	for _, role := range []models.ApplicationRole{models.ApplicationRoleMentor, "owner"} {
		svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, termRepo, &stubProgRepo{})
		if _, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{UserID: "u1", Role: role}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("role %q: err = %v; want ErrInvalidInput", role, err)
		}
	}

	var created models.ApplicationRole
	repo := &stubAppRepo{create: func(_ context.Context, _ string, in models.ApplicationCreateInput) (*models.Application, error) {
		created = in.Role
		return &models.Application{ID: in.ID, Role: in.Role}, nil
	}}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	if _, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{UserID: "u1"}); err != nil || created != models.ApplicationRoleMentee {
		t.Fatalf("no role: err = %v, created role %q; want a mentee application", err, created)
	}
}

// TestApplicationService_Create_WindowMatchesAcceptsApplications pins Create to
// ProgramTerm.AcceptsApplications, which backs the apply-target read: the apply
// page must never say a term takes applications when submitting would reject it.
func TestApplicationService_Create_WindowMatchesAcceptsApplications(t *testing.T) {
	now := time.Now()
	past := now.Add(-48 * time.Hour)
	future := now.Add(48 * time.Hour)
	cases := map[string]*models.ProgramTerm{
		"open inside window":    {ID: "t1", Status: models.ProgramTermStatusOpen, ApplicationStartDate: &past, ApplicationEndDate: &future},
		"open before window":    {ID: "t1", Status: models.ProgramTermStatusOpen, ApplicationStartDate: &future, ApplicationEndDate: &future},
		"open after window":     {ID: "t1", Status: models.ProgramTermStatusOpen, ApplicationStartDate: &past, ApplicationEndDate: &past},
		"open without window":   {ID: "t1", Status: models.ProgramTermStatusOpen},
		"open without start":    {ID: "t1", Status: models.ProgramTermStatusOpen, ApplicationEndDate: &future},
		"open without end":      {ID: "t1", Status: models.ProgramTermStatusOpen, ApplicationStartDate: &past},
		"closed inside window":  {ID: "t1", Status: models.ProgramTermStatusClosed, ApplicationStartDate: &past, ApplicationEndDate: &future},
		"deleted without dates": {ID: "t1", Status: models.ProgramTermStatusDeleted},
	}
	for name, term := range cases {
		t.Run(name, func(t *testing.T) {
			termRepo := &stubTermRepo{
				getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) { return term, nil },
			}
			svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, termRepo, &stubProgRepo{})
			_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
			if rejected := errors.Is(err, domain.ErrIneligible); rejected == term.AcceptsApplications(now) {
				t.Fatalf("Create rejected = %v but AcceptsApplications = %v (err %v)", rejected, term.AcceptsApplications(now), err)
			}
		})
	}
}

func TestApplicationService_Create_DuplicateActive_Blocked(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
		return openTerm(time.Now()), nil
	}}
	repo := &stubAppRepo{
		findByTermAndUser: func(_ context.Context, _, _ string, _ models.ApplicationRole) (*models.Application, error) {
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
		findByTermAndUser: func(_ context.Context, _, _ string, _ models.ApplicationRole) (*models.Application, error) {
			return &models.Application{ID: "existing", Status: "declined"}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestApplicationService_Create_WithdrawnReapply_ReappliesFromWithdrawn(t *testing.T) {
	var reappliedFrom string
	termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
		return openTerm(time.Now()), nil
	}}
	repo := &stubAppRepo{
		findByTermAndUser: func(_ context.Context, _, _ string, _ models.ApplicationRole) (*models.Application, error) {
			return &models.Application{ID: "old", Status: "withdrawn"}, nil
		},
		reapply: func(_ context.Context, oldID, _ string, _ models.ApplicationCreateInput) (*models.Application, error) {
			reappliedFrom = oldID
			return &models.Application{ID: "new", Status: "pending"}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: "mentee"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if reappliedFrom != "old" {
		t.Errorf("reapply was not made from the withdrawn application; got oldID=%q", reappliedFrom)
	}
}

func TestApplicationService_Create_WithdrawnReapply_Limit(t *testing.T) {
	cases := []struct {
		withdrawn int
		wantErr   bool
	}{
		{withdrawn: 2, wantErr: false},
		{withdrawn: 3, wantErr: true},
		{withdrawn: 4, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d withdrawn", tc.withdrawn), func(t *testing.T) {
			termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
				return openTerm(time.Now()), nil
			}}
			reapplied := false
			repo := &stubAppRepo{
				findByTermAndUser: func(_ context.Context, _, _ string, _ models.ApplicationRole) (*models.Application, error) {
					return &models.Application{ID: "old", Status: models.ApplicationStatusWithdrawn}, nil
				},
				countWithdrawn: func(_ context.Context, termID, userID string, role models.ApplicationRole) (int, error) {
					if termID != "t1" || userID != "u1" || role != models.ApplicationRoleMentee {
						t.Fatalf("countWithdrawn arguments = term %q, user %q, role %q", termID, userID, role)
					}
					return tc.withdrawn, nil
				},
				reapply: func(_ context.Context, _, _ string, _ models.ApplicationCreateInput) (*models.Application, error) {
					reapplied = true
					return &models.Application{ID: "new", Status: models.ApplicationStatusPending}, nil
				},
			}
			svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{})
			_, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: models.ApplicationRoleMentee})
			if tc.wantErr {
				if !errors.Is(err, domain.ErrIneligible) || !strings.Contains(err.Error(), "reapplication limit reached") {
					t.Fatalf("err = %v; want ErrIneligible naming the reapplication limit", err)
				}
				if reapplied {
					t.Fatal("a new application was created past the reapplication limit")
				}
				return
			}
			if err != nil || !reapplied {
				t.Fatalf("err = %v, reapplied = %v; want a new application", err, reapplied)
			}
		})
	}
}

func TestApplicationService_Create_CountWithdrawnError(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
		return openTerm(time.Now()), nil
	}}
	countErr := errors.New("db down")
	repo := &stubAppRepo{
		findByTermAndUser: func(_ context.Context, _, _ string, _ models.ApplicationRole) (*models.Application, error) {
			return &models.Application{ID: "old", Status: models.ApplicationStatusWithdrawn}, nil
		},
		countWithdrawn: func(context.Context, string, string, models.ApplicationRole) (int, error) { return 0, countErr },
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, &stubProgRepo{})
	if _, err := svc.Create(context.Background(), "t1", models.ApplicationCreateInput{UserID: "u1", Role: models.ApplicationRoleMentee}); !errors.Is(err, countErr) {
		t.Fatalf("err = %v; want the count error", err)
	}
}

func TestApplicationService_Create_WithdrawnReapply_ClonesPrerequisiteTasks(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
		term := openTerm(time.Now())
		term.ProgramID = "program-1"
		return term, nil
	}}
	repo := &stubAppRepo{
		findByTermAndUser: func(_ context.Context, _, _ string, _ models.ApplicationRole) (*models.Application, error) {
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
		if tasks[0].SubmitFile == nil || *tasks[0].SubmitFile != "required" || tasks[0].DueDate == nil || *tasks[0].DueDate != "2026-05-01" || !tasks[0].Custom {
			t.Fatalf("task submit_file %v, due_date %v, custom %v; want the template's required, 2026-05-01 and true", tasks[0].SubmitFile, tasks[0].DueDate, tasks[0].Custom)
		}
		return &models.Application{ID: "new", ProgramTermID: "term-1", UserID: "u1", Status: models.ApplicationStatusPending, Role: models.ApplicationRoleMentee}, nil
	}
	progRepo := &stubProgRepo{getByID: func(_ context.Context, _ string) (*models.Program, error) {
		return &models.Program{TaskTemplates: json.RawMessage(`[{"name":"Code of Conduct","description":"Read it","submitFile":"required","dueDate":"2026-05-01","custom":true}]`)}, nil
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

func TestApplicationService_Create_ClonesOldFormatTaskTemplates(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
		term := openTerm(time.Now())
		term.ProgramID = "program-1"
		return term, nil
	}}
	repo := &stubAppRepo{
		findByTermAndUser: func(_ context.Context, _, _ string, _ models.ApplicationRole) (*models.Application, error) {
			return &models.Application{ID: "old", Status: models.ApplicationStatusWithdrawn}, nil
		},
	}
	var cloned []models.TaskCreateInput
	repo.reapplyWithTasks = func(_ context.Context, _ string, _ string, _ models.ApplicationCreateInput, tasks []models.TaskCreateInput) (*models.Application, error) {
		cloned = tasks
		return &models.Application{ID: "new", ProgramTermID: "term-1", UserID: "u1", Status: models.ApplicationStatusPending, Role: models.ApplicationRoleMentee}, nil
	}
	// Saved before the migration and update normalized templates: string custom flags, a blank name,
	// and blank or invalid submitFile and dueDate values.
	progRepo := &stubProgRepo{getByID: func(_ context.Context, _ string) (*models.Program, error) {
		return &models.Program{TaskTemplates: json.RawMessage(`[
			{"name":"Essay","submitFile":"","dueDate":"","custom":"true"},
			{"name":"  ","submitFile":"required","custom":"true"},
			{"name":"Resume","submitFile":"yes","dueDate":"soon","custom":"false"},
			{"name":"Bio","submitFile":"required","dueDate":"2026-05-01","custom":"undefined"}
		]`)}, nil
	}}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, termRepo, progRepo)
	if _, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{UserID: "u1", Role: models.ApplicationRoleMentee}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(cloned) != 3 {
		t.Fatalf("cloned %d tasks; want 3", len(cloned))
	}
	for i, task := range cloned[:2] {
		if task.SubmitFile != nil || task.DueDate != nil {
			t.Fatalf("task %d submit_file %v, due_date %v; want blank and invalid values cleared", i, task.SubmitFile, task.DueDate)
		}
	}
	if cloned[2].SubmitFile == nil || *cloned[2].SubmitFile != "required" || cloned[2].DueDate == nil || *cloned[2].DueDate != "2026-05-01" {
		t.Fatalf("task 2 submit_file %v, due_date %v; want valid values kept", cloned[2].SubmitFile, cloned[2].DueDate)
	}
	if !cloned[0].Custom || cloned[1].Custom || cloned[2].Custom {
		t.Fatalf("custom = %v, %v, %v; want only the \"true\" template custom", cloned[0].Custom, cloned[1].Custom, cloned[2].Custom)
	}
}

func TestApplicationService_Update_ValidTransition(t *testing.T) {
	var expected *models.ApplicationStatus
	repo := &stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, Status: "pending"}, nil
		},
		update: func(_ context.Context, id string, in models.ApplicationUpdateInput) (*models.Application, error) {
			expected = in.ExpectedStatus
			return &models.Application{ID: id}, nil
		},
	}
	svc := newApplicationSvc(repo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
	next := models.ApplicationStatusAccepted
	attType := models.AttendanceTypeFullTime
	_, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next, AttendanceType: &attType, ActorID: "reviewer"})
	if err != nil {
		t.Errorf("expected valid transition pending→accepted, got %v", err)
	}
	// The repository must apply the change only while the row is still in the status that was validated.
	if expected == nil || *expected != models.ApplicationStatusPending {
		t.Errorf("ExpectedStatus = %v; want pending", expected)
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
	svc := service.NewApplicationService(repo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{}, approvedProgramAdmin(), menteeProfiles(1), notifier)
	next := models.ApplicationStatusAccepted
	attType := models.AttendanceTypeFullTime
	if _, err := svc.Update(context.Background(), "app-mentee-1", models.ApplicationUpdateInput{Status: &next, AttendanceType: &attType, ActorID: "reviewer"}); err != nil {
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
	if _, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next, ActorID: "reviewer"}); err != nil {
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
	_, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next, ActorID: "reviewer"})
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
	_, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next, ActorID: "reviewer"})
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
	active := models.ProgramMemberStatusApproved
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
	active := models.ProgramMemberStatusApproved
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

func TestApplicationService_ListByProgram_StatusFilter(t *testing.T) {
	for _, status := range []models.ProgramApplicationStatus{
		models.ProgramApplicationStatusApplied,
		models.ProgramApplicationStatusTasksSubmitted,
		models.ProgramApplicationStatus(models.ApplicationStatusAccepted),
	} {
		var got models.ProgramApplicationStatus
		svc := newApplicationSvc(&stubAppRepo{
			listByProgram: func(_ context.Context, _ string, f models.ProgramApplicationFilter) ([]*models.ProgramApplicationRow, *models.PaginationMeta, error) {
				got = f.Status
				return []*models.ProgramApplicationRow{}, &models.PaginationMeta{}, nil
			},
		}, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
		if _, _, err := svc.ListByProgram(context.Background(), "prog-1", models.ProgramApplicationFilter{Status: status}); err != nil || got != status {
			t.Fatalf("ListByProgram(status %q) passed %q, err = %v; want it passed through", status, got, err)
		}
	}

	svc := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{})
	if _, _, err := svc.ListByProgram(context.Background(), "prog-1", models.ProgramApplicationFilter{Status: "submitted"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("ListByProgram(unknown status) err = %v; want ErrInvalidInput", err)
	}
}

// ── notifications ───────────────────────────────────────────────────────────

func newApplicationSvcWithNotifier(appRepo *stubAppRepo, termRepo *stubTermRepo, n *stubNotifier) *service.ApplicationService {
	return service.NewApplicationService(appRepo, &stubTaskRepo{}, termRepo, &stubProgRepo{}, approvedProgramAdmin(), menteeProfiles(1), n)
}

// menteeDecisionCalls counts the mentee notifications an application status change can send.
type menteeDecisionCalls struct{ accepted, declined, withdrawn, graduated int }

func (n *stubNotifier) menteeDecisions() menteeDecisionCalls {
	return menteeDecisionCalls{n.menteeAcceptedCalls, n.menteeDeclinedCalls, n.menteeWithdrawnCalls, n.menteeGraduatedCalls}
}

func TestApplicationService_Create_NotifiesMentee(t *testing.T) {
	for _, tc := range []struct {
		name                string
		role                models.ApplicationRole
		existing            *models.Application
		want, wantRequested int
	}{
		{name: "new mentee application", role: models.ApplicationRoleMentee, want: 1},
		{name: "mentee reapplies after withdrawing", role: models.ApplicationRoleMentee, existing: &models.Application{ID: "old", Status: models.ApplicationStatusWithdrawn}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			termRepo := &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) {
				return openTerm(time.Now()), nil
			}}
			repo := &stubAppRepo{findByTermAndUser: func(context.Context, string, string, models.ApplicationRole) (*models.Application, error) {
				return tc.existing, nil
			}}
			n := &stubNotifier{}
			if _, err := newApplicationSvcWithNotifier(repo, termRepo, n).Create(context.Background(), "term-1", models.ApplicationCreateInput{UserID: "u1", Role: tc.role}); err != nil {
				t.Fatalf("Create: %v", err)
			}
			if n.applicationReceivedCalls != tc.want || n.adminMentorRequestedCalls != tc.wantRequested {
				t.Fatalf("received / mentor requested notifications = %d / %d; want %d / %d", n.applicationReceivedCalls, n.adminMentorRequestedCalls, tc.want, tc.wantRequested)
			}
		})
	}
}

func TestApplicationService_Create_RepoErrorSendsNothing(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) {
		return openTerm(time.Now()), nil
	}}
	repo := &stubAppRepo{create: func(context.Context, string, models.ApplicationCreateInput) (*models.Application, error) {
		return nil, errors.New("db down")
	}}
	n := &stubNotifier{}
	if _, err := newApplicationSvcWithNotifier(repo, termRepo, n).Create(context.Background(), "term-1", models.ApplicationCreateInput{UserID: "u1", Role: models.ApplicationRoleMentee}); err == nil {
		t.Fatal("Create succeeded despite the repository error")
	}
	if n.applicationReceivedCalls != 0 {
		t.Fatalf("application received notifications = %d; want 0", n.applicationReceivedCalls)
	}
}

func TestApplicationService_Update_NotifiesMenteeOfStatusChange(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to models.ApplicationStatus
		role     models.ApplicationRole
		actor    string
		want     menteeDecisionCalls
	}{
		{name: "declined while pending", from: models.ApplicationStatusPending, to: models.ApplicationStatusDeclined, role: models.ApplicationRoleMentee, actor: "reviewer", want: menteeDecisionCalls{declined: 1}},
		{name: "declined after acceptance", from: models.ApplicationStatusAccepted, to: models.ApplicationStatusDeclined, role: models.ApplicationRoleMentee, actor: "reviewer", want: menteeDecisionCalls{declined: 1}},
		{name: "self-withdrawn", from: models.ApplicationStatusPending, to: models.ApplicationStatusWithdrawn, role: models.ApplicationRoleMentee, actor: "mentee-1", want: menteeDecisionCalls{withdrawn: 1}},
		{name: "graduated", from: models.ApplicationStatusAccepted, to: models.ApplicationStatusGraduated, role: models.ApplicationRoleMentee, actor: "reviewer", want: menteeDecisionCalls{graduated: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubAppRepo{
				getByID: func(_ context.Context, id string) (*models.Application, error) {
					return &models.Application{ID: id, Status: tc.from, ProgramTermID: "term-1", UserID: "mentee-1", Role: tc.role}, nil
				},
				update: func(_ context.Context, id string, _ models.ApplicationUpdateInput) (*models.Application, error) {
					return &models.Application{ID: id, Status: tc.to, ProgramTermID: "term-1", UserID: "mentee-1", Role: tc.role}, nil
				},
			}
			n := &stubNotifier{}
			next := tc.to
			if _, err := newApplicationSvcWithNotifier(repo, &stubTermRepo{}, n).Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next, ActorID: tc.actor}); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if got := n.menteeDecisions(); got != tc.want {
				t.Fatalf("notifications = %+v; want %+v", got, tc.want)
			}
		})
	}
}

func TestApplicationService_Update_NotifiesOfMentorApplicationDecision(t *testing.T) {
	for _, tc := range []struct {
		name         string
		from, to     models.ApplicationStatus
		actor        string
		termErr      error
		wantWithdrew int
		wantDeclined int
	}{
		{name: "withdrawn tells admins", from: models.ApplicationStatusPending, to: models.ApplicationStatusWithdrawn, actor: "mentor-1", wantWithdrew: 1},
		{name: "declined while pending tells the mentor", from: models.ApplicationStatusPending, to: models.ApplicationStatusDeclined, actor: "reviewer", wantDeclined: 1},
		{name: "declined after acceptance sends nothing", from: models.ApplicationStatusAccepted, to: models.ApplicationStatusDeclined, actor: "reviewer"},
		{name: "withdrawn, term lookup fails after the write", from: models.ApplicationStatusPending, to: models.ApplicationStatusWithdrawn, actor: "mentor-1", termErr: errors.New("db down")},
		{name: "declined, term lookup fails after the write", from: models.ApplicationStatusPending, to: models.ApplicationStatusDeclined, actor: "reviewer", termErr: errors.New("db down")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			written := false
			repo := &stubAppRepo{
				getByID: func(_ context.Context, id string) (*models.Application, error) {
					return &models.Application{ID: id, Status: tc.from, ProgramTermID: "term-1", UserID: "mentor-1", Role: models.ApplicationRoleMentor}, nil
				},
				update: func(_ context.Context, id string, _ models.ApplicationUpdateInput) (*models.Application, error) {
					written = true
					return &models.Application{ID: id, Status: tc.to, ProgramTermID: "term-1", UserID: "mentor-1", Role: models.ApplicationRoleMentor}, nil
				},
			}
			// The program admin check also reads the term, so fail only the lookup after the write.
			termRepo := &stubTermRepo{getByID: func(_ context.Context, id string) (*models.ProgramTerm, error) {
				if written && tc.termErr != nil {
					return nil, tc.termErr
				}
				return &models.ProgramTerm{ID: id, ProgramID: "prog-1"}, nil
			}}
			n := &stubNotifier{}
			next := tc.to
			if _, err := newApplicationSvcWithNotifier(repo, termRepo, n).Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next, ActorID: tc.actor}); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if n.adminMentorWithdrewCalls != tc.wantWithdrew || n.mentorDeclinedCalls != tc.wantDeclined {
				t.Fatalf("mentor withdrew / mentor declined notifications = %d / %d; want %d / %d", n.adminMentorWithdrewCalls, n.mentorDeclinedCalls, tc.wantWithdrew, tc.wantDeclined)
			}
			if got := n.menteeDecisions(); got != (menteeDecisionCalls{}) {
				t.Fatalf("mentee notifications = %+v; want none", got)
			}
		})
	}
}

func TestApplicationService_Update_RepoErrorSendsNothing(t *testing.T) {
	repo := &stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, Status: models.ApplicationStatusPending, UserID: "mentee-1", Role: models.ApplicationRoleMentee}, nil
		},
		update: func(context.Context, string, models.ApplicationUpdateInput) (*models.Application, error) {
			return nil, domain.ErrInvalidStateTransition
		},
	}
	n := &stubNotifier{}
	next := models.ApplicationStatusDeclined
	if _, err := newApplicationSvcWithNotifier(repo, &stubTermRepo{}, n).Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &next, ActorID: "reviewer"}); err == nil {
		t.Fatal("Update succeeded despite the repository error")
	}
	if got := n.menteeDecisions(); got != (menteeDecisionCalls{}) {
		t.Fatalf("notifications = %+v; want none", got)
	}
}

func TestApplicationService_AssistedWithdrawal_NotifiesMentee(t *testing.T) {
	for _, tc := range []struct {
		name string
		from models.ApplicationStatus
		role models.ApplicationRole
		want int
	}{
		{name: "pending mentee application", from: models.ApplicationStatusPending, role: models.ApplicationRoleMentee, want: 1},
		{name: "already withdrawn", from: models.ApplicationStatusWithdrawn, role: models.ApplicationRoleMentee, want: 0},
		{name: "mentor application", from: models.ApplicationStatusPending, role: models.ApplicationRoleMentor, want: 0},
	} {
		for path, withdraw := range map[string]func(*service.ApplicationService) (*models.Application, error){
			"program admin": func(s *service.ApplicationService) (*models.Application, error) {
				return s.WithdrawForMentee(context.Background(), "app-1", "admin-1")
			},
			"gateway": func(s *service.ApplicationService) (*models.Application, error) {
				return s.WithdrawForMenteeAfterGatewayAuthorization(context.Background(), "app-1")
			},
		} {
			t.Run(tc.name+" via "+path, func(t *testing.T) {
				var expected *models.ApplicationStatus
				repo := &stubAppRepo{
					getByID: func(_ context.Context, id string) (*models.Application, error) {
						return &models.Application{ID: id, Status: tc.from, ProgramTermID: "term-1", UserID: "mentee-1", Role: tc.role}, nil
					},
					update: func(_ context.Context, id string, in models.ApplicationUpdateInput) (*models.Application, error) {
						expected = in.ExpectedStatus
						return &models.Application{ID: id, Status: *in.Status, ProgramTermID: "term-1", UserID: "mentee-1", Role: tc.role}, nil
					},
				}
				n := &stubNotifier{}
				if _, err := withdraw(newApplicationSvcWithNotifier(repo, &stubTermRepo{}, n)); err != nil {
					t.Fatalf("withdraw: %v", err)
				}
				if expected == nil || *expected != tc.from {
					t.Fatalf("ExpectedStatus = %v; want %q", expected, tc.from)
				}
				if n.menteeWithdrawnCalls != tc.want {
					t.Fatalf("withdrawn notifications = %d; want %d", n.menteeWithdrawnCalls, tc.want)
				}
			})
		}
	}
}

// A declined or graduated application is a final outcome: withdrawing it would let the mentee
// reapply to the term, which a decline forbids.
func TestApplicationService_AssistedWithdrawal_RefusesFinalOutcomes(t *testing.T) {
	for _, from := range []models.ApplicationStatus{models.ApplicationStatusDeclined, models.ApplicationStatusGraduated} {
		repo := &stubAppRepo{
			getByID: func(_ context.Context, id string) (*models.Application, error) {
				return &models.Application{ID: id, Status: from, ProgramTermID: "term-1", UserID: "mentee-1", Role: models.ApplicationRoleMentee}, nil
			},
			update: func(context.Context, string, models.ApplicationUpdateInput) (*models.Application, error) {
				t.Fatalf("a %s application was withdrawn", from)
				return nil, nil
			},
		}
		n := &stubNotifier{}
		svc := newApplicationSvcWithNotifier(repo, &stubTermRepo{}, n)
		if _, err := svc.WithdrawForMentee(context.Background(), "app-1", "admin-1"); !errors.Is(err, domain.ErrInvalidStateTransition) {
			t.Fatalf("from %s via program admin: err = %v; want ErrInvalidStateTransition", from, err)
		}
		if _, err := svc.WithdrawForMenteeAfterGatewayAuthorization(context.Background(), "app-1"); !errors.Is(err, domain.ErrInvalidStateTransition) {
			t.Fatalf("from %s via gateway: err = %v; want ErrInvalidStateTransition", from, err)
		}
		if n.menteeWithdrawnCalls != 0 {
			t.Fatalf("from %s: withdrawn notifications = %d; want 0", from, n.menteeWithdrawnCalls)
		}
	}
}

func TestApplicationService_AssistedWithdrawal_LostRaceSendsNothing(t *testing.T) {
	repo := &stubAppRepo{
		getByID: func(_ context.Context, id string) (*models.Application, error) {
			return &models.Application{ID: id, Status: models.ApplicationStatusPending, ProgramTermID: "term-1", UserID: "mentee-1", Role: models.ApplicationRoleMentee}, nil
		},
		// The row changed between the read and the write, so the conditional update is rejected.
		update: func(context.Context, string, models.ApplicationUpdateInput) (*models.Application, error) {
			return nil, domain.ErrInvalidStateTransition
		},
	}
	n := &stubNotifier{}
	_, err := newApplicationSvcWithNotifier(repo, &stubTermRepo{}, n).WithdrawForMenteeAfterGatewayAuthorization(context.Background(), "app-1")
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("err = %v; want ErrInvalidStateTransition", err)
	}
	if n.menteeWithdrawnCalls != 0 {
		t.Fatalf("withdrawn notifications = %d; want 0", n.menteeWithdrawnCalls)
	}
}

func TestApplicationService_BulkDeclineByTerm_NotifiesEachMentee(t *testing.T) {
	repo := &stubAppRepo{bulkDecline: func(context.Context, string) ([]*models.Application, error) {
		return []*models.Application{
			{ID: "a1", Role: models.ApplicationRoleMentee},
			{ID: "a2", Role: models.ApplicationRoleMentor},
			{ID: "a3", Role: models.ApplicationRoleMentee},
		}, nil
	}}
	n := &stubNotifier{}
	count, err := newApplicationSvcWithNotifier(repo, &stubTermRepo{}, n).BulkDeclineByTerm(context.Background(), "term-1", "admin-1")
	if err != nil {
		t.Fatalf("BulkDeclineByTerm: %v", err)
	}
	if count != 3 || n.menteeDeclinedCalls != 2 {
		t.Fatalf("count = %d, declined notifications = %d; want 3 declined and 2 mentees notified", count, n.menteeDeclinedCalls)
	}
}

// ── program admin decisions ─────────────────────────────────────────────────

// Off the gateway the service repeats the gateway's manager check: a mentor reviews an application
// but does not decide it, and only a program admin deletes or bulk-declines.
func TestApplicationService_AdminDecisionsRefuseMentors(t *testing.T) {
	application := func(_ context.Context, id string) (*models.Application, error) {
		return &models.Application{ID: id, Status: models.ApplicationStatusPending, ProgramTermID: "term-1", UserID: "mentee-1", Role: models.ApplicationRoleMentee}, nil
	}
	repo := &stubAppRepo{
		getByID: application,
		update: func(_ context.Context, id string, in models.ApplicationUpdateInput) (*models.Application, error) {
			if in.Status != nil {
				t.Fatal("a mentor changed an application's status")
			}
			return application(context.Background(), id)
		},
		delete: func(context.Context, string) error {
			t.Fatal("a mentor deleted an application")
			return nil
		},
		bulkDecline: func(context.Context, string) ([]*models.Application, error) {
			t.Fatal("a mentor bulk-declined a term")
			return nil, nil
		},
	}
	svc := service.NewApplicationService(repo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{}, approvedMentor(), menteeProfiles(1), &stubNotifier{})

	declined := models.ApplicationStatusDeclined
	if _, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{Status: &declined, ActorID: "mentor-1"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("status change: err = %v; want ErrForbidden", err)
	}
	if err := svc.Delete(context.Background(), "app-1", "mentor-1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("delete: err = %v; want ErrForbidden", err)
	}
	if _, err := svc.BulkDeclineByTerm(context.Background(), "term-1", "mentor-1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bulk decline: err = %v; want ErrForbidden", err)
	}
	// Reviewing stays open to mentors.
	note := "strong candidate"
	if _, err := svc.Update(context.Background(), "app-1", models.ApplicationUpdateInput{ReviewerNote: &note, ActorID: "mentor-1"}); err != nil {
		t.Fatalf("reviewer note: %v", err)
	}
}

func TestApplicationService_Delete_ProgramAdmin(t *testing.T) {
	deleted := ""
	repo := &stubAppRepo{delete: func(_ context.Context, id string) error {
		deleted = id
		return nil
	}}
	if err := newApplicationSvc(repo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{}).Delete(context.Background(), "app-1", "admin-1"); err != nil || deleted != "app-1" {
		t.Fatalf("err = %v, deleted %q; want app-1 deleted", err, deleted)
	}
}

func TestApplicationService_Create_RequiresUserID(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) {
		t.Fatal("term must not be read without an applicant")
		return nil, nil
	}}
	_, err := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, termRepo, &stubProgRepo{}).Create(context.Background(), "term-1", models.ApplicationCreateInput{Role: models.ApplicationRoleMentee})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v; want ErrInvalidInput", err)
	}
}

func TestApplicationService_Create_PropagatesErrors(t *testing.T) {
	repoErr := errors.New("db down")
	withdrawn := func(context.Context, string, string, models.ApplicationRole) (*models.Application, error) {
		return &models.Application{ID: "old", Status: models.ApplicationStatusWithdrawn}, nil
	}
	failingProgram := &stubProgRepo{getByID: func(context.Context, string) (*models.Program, error) { return nil, repoErr }}
	for label, tc := range map[string]struct {
		appRepo  *stubAppRepo
		termRepo *stubTermRepo
		progRepo *stubProgRepo
		profiles *stubUserProfileRepo
	}{
		"load term": {termRepo: &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) { return nil, repoErr }}},
		"count mentee profiles": {profiles: &stubUserProfileRepo{countActiveMenteeProfiles: func(context.Context, string) (int, error) {
			return 0, repoErr
		}}},
		"find existing application": {appRepo: &stubAppRepo{findByTermAndUser: func(context.Context, string, string, models.ApplicationRole) (*models.Application, error) {
			return nil, repoErr
		}}},
		"load task templates":                  {progRepo: failingProgram},
		"load task templates on reapplication": {appRepo: &stubAppRepo{findByTermAndUser: withdrawn}, progRepo: failingProgram},
		"write reapplication": {appRepo: &stubAppRepo{
			findByTermAndUser: withdrawn,
			reapplyWithTasks: func(context.Context, string, string, models.ApplicationCreateInput, []models.TaskCreateInput) (*models.Application, error) {
				return nil, repoErr
			},
		}},
	} {
		t.Run(label, func(t *testing.T) {
			if tc.appRepo == nil {
				tc.appRepo = &stubAppRepo{}
			}
			if tc.termRepo == nil {
				tc.termRepo = &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) { return openTerm(time.Now()), nil }}
			}
			if tc.progRepo == nil {
				tc.progRepo = &stubProgRepo{}
			}
			if tc.profiles == nil {
				tc.profiles = menteeProfiles(1)
			}
			n := &stubNotifier{}
			svc := service.NewApplicationService(tc.appRepo, &stubTaskRepo{}, tc.termRepo, tc.progRepo, approvedProgramAdmin(), tc.profiles, n)
			if _, err := svc.Create(context.Background(), "term-1", models.ApplicationCreateInput{UserID: "u1", Role: models.ApplicationRoleMentee}); !errors.Is(err, repoErr) {
				t.Fatalf("err = %v; want the repository error", err)
			}
			if n.applicationReceivedCalls != 0 {
				t.Errorf("application received notifications = %d; want 0", n.applicationReceivedCalls)
			}
		})
	}
}

func TestApplicationService_Create_RejectsCorruptTaskTemplates(t *testing.T) {
	termRepo := &stubTermRepo{getByID: func(context.Context, string) (*models.ProgramTerm, error) { return openTerm(time.Now()), nil }}
	progRepo := &stubProgRepo{getByID: func(_ context.Context, id string) (*models.Program, error) {
		return &models.Program{ID: id, TaskTemplates: json.RawMessage(`{"not":"a list"}`)}, nil
	}}
	appRepo := &stubAppRepo{create: func(context.Context, string, models.ApplicationCreateInput) (*models.Application, error) {
		t.Fatal("an application must not be created without its prerequisite tasks")
		return nil, nil
	}}
	_, err := newApplicationSvc(appRepo, &stubTaskRepo{}, termRepo, progRepo).Create(context.Background(), "term-1", models.ApplicationCreateInput{UserID: "u1", Role: models.ApplicationRoleMentee})
	if err == nil || !strings.Contains(err.Error(), "decode program task templates") {
		t.Fatalf("err = %v; want a task template decode error", err)
	}
}

func TestApplicationService_Update_AuthorizationFailures(t *testing.T) {
	repoErr := errors.New("db down")
	accepted := models.ApplicationStatusAccepted
	note := "looks good"
	failingMembers := &stubMemberRepo{
		findActiveReviewer: func(context.Context, string, string) (*models.ProgramMember, error) { return nil, repoErr },
		findActiveAdmin:    func(context.Context, string, string) (*models.ProgramMember, error) { return nil, repoErr },
	}
	for label, tc := range map[string]struct {
		appRepo *stubAppRepo
		members *stubMemberRepo
		input   models.ApplicationUpdateInput
		want    error
		// message names the check expected to refuse; several return ErrForbidden.
		message string
	}{
		"application lookup fails": {
			appRepo: &stubAppRepo{getByID: func(context.Context, string) (*models.Application, error) { return nil, repoErr }},
			input:   models.ApplicationUpdateInput{Status: &accepted, ActorID: "admin-1"},
			want:    repoErr,
		},
		"status change without an actor": {input: models.ApplicationUpdateInput{Status: &accepted}, want: domain.ErrForbidden, message: "actor identity is required"},
		"reviewer note without an actor": {input: models.ApplicationUpdateInput{ReviewerNote: &note}, want: domain.ErrForbidden, message: "actor identity is required"},
		"reviewer lookup fails":          {members: failingMembers, input: models.ApplicationUpdateInput{ReviewerNote: &note, ActorID: "mentor-1"}, want: repoErr},
		"program admin lookup fails":     {members: failingMembers, input: models.ApplicationUpdateInput{Status: &accepted, ActorID: "admin-1"}, want: repoErr},
		"mentor cannot decide":           {members: approvedMentor(), input: models.ApplicationUpdateInput{Status: &accepted, ActorID: "mentor-1"}, want: domain.ErrForbidden, message: "active program_admin"},
	} {
		t.Run(label, func(t *testing.T) {
			if tc.appRepo == nil {
				tc.appRepo = &stubAppRepo{}
			}
			tc.appRepo.update = func(context.Context, string, models.ApplicationUpdateInput) (*models.Application, error) {
				t.Fatal("an unauthorized update must not be written")
				return nil, nil
			}
			if tc.members == nil {
				tc.members = approvedProgramAdmin()
			}
			svc := newApplicationSvcWithMember(tc.appRepo, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{}, tc.members)
			_, err := svc.Update(context.Background(), "app-1", tc.input)
			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("err = %v; want %v mentioning %q", err, tc.want, tc.message)
			}
		})
	}
}
