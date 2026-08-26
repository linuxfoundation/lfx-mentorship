// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

// stubMemberRepo implements domain.ProgramMemberRepository for task tests.
type stubMemberRepo struct {
	getByID           func(context.Context, string) (*models.ProgramMember, error)
	findByProgramUser func(context.Context, string, string) (*models.ProgramMember, error)
	listByProgram     func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error)
	create            func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error)
	update            func(context.Context, string, models.ProgramMemberUpdateInput) (*models.ProgramMember, error)
	delete            func(context.Context, string) error
}

func (m *stubMemberRepo) GetByID(ctx context.Context, id string) (*models.ProgramMember, error) {
	if m.getByID != nil {
		return m.getByID(ctx, id)
	}
	return &models.ProgramMember{ID: id}, nil
}
func (m *stubMemberRepo) FindByProgramAndUser(ctx context.Context, programID, userID string) (*models.ProgramMember, error) {
	if m.findByProgramUser != nil {
		return m.findByProgramUser(ctx, programID, userID)
	}
	return nil, domain.ErrProgramMemberNotFound
}
func (m *stubMemberRepo) ListByProgram(ctx context.Context, programID string, f models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
	if m.listByProgram != nil {
		return m.listByProgram(ctx, programID, f)
	}
	return nil, &models.PaginationMeta{}, nil
}
func (m *stubMemberRepo) Create(ctx context.Context, programID string, in models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
	if m.create != nil {
		return m.create(ctx, programID, in)
	}
	return &models.ProgramMember{}, nil
}
func (m *stubMemberRepo) Update(ctx context.Context, id string, in models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
	if m.update != nil {
		return m.update(ctx, id, in)
	}
	return &models.ProgramMember{ID: id}, nil
}
func (m *stubMemberRepo) Delete(ctx context.Context, id string) error {
	if m.delete != nil {
		return m.delete(ctx, id)
	}
	return nil
}

// newTaskSvc builds a TaskService wired to the provided stubs.
func newTaskSvc(taskRepo *stubTaskRepo, appRepo *stubAppRepo, termRepo *stubTermRepo, memberRepo *stubMemberRepo) *service.TaskService {
	return service.NewTaskService(taskRepo, appRepo, termRepo, memberRepo, &stubNotifier{})
}

// ── task state machine ───────────────────────────────────────────────────────

func TestTaskService_Update_Assignee_CanMarkInProgress(t *testing.T) {
	appID := "app-1"
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "user-1", Status: "incomplete", ApplicationID: &appID}, nil
		},
		update: func(_ context.Context, id string, _ models.TaskUpdateInput) (*models.Task, error) {
			return &models.Task{ID: id, Status: "in_progress"}, nil
		},
	}
	svc := newTaskSvc(taskRepo, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{})
	next := models.TaskStatusInProgress
	_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{
		Status:  &next,
		ActorID: "user-1",
	})
	if err != nil {
		t.Errorf("assignee should mark in_progress: %v", err)
	}
}

func TestTaskService_Update_Assignee_CannotMarkComplete(t *testing.T) {
	appID := "app-1"
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "user-1", Status: "submitted", ApplicationID: &appID}, nil
		},
	}
	svc := newTaskSvc(taskRepo, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{})
	next := models.TaskStatusComplete
	_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{
		Status:  &next,
		ActorID: "user-1",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("expected ErrForbidden for assignee marking complete, got %v", err)
	}
}

func TestTaskService_Update_Assignee_CannotMarkIncomplete(t *testing.T) {
	appID := "app-1"
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "user-1", Status: "complete", ApplicationID: &appID}, nil
		},
	}
	svc := newTaskSvc(taskRepo, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{})
	next := models.TaskStatusIncomplete
	_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{
		Status:  &next,
		ActorID: "user-1",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("expected ErrForbidden for assignee marking incomplete, got %v", err)
	}
}

func TestTaskService_Update_NonAssignee_CannotMarkInProgress(t *testing.T) {
	appID := "app-1"
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "user-1", Status: "incomplete", ApplicationID: &appID}, nil
		},
	}
	svc := newTaskSvc(taskRepo, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{})
	next := models.TaskStatusInProgress
	_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{
		Status:  &next,
		ActorID: "reviewer-99",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("expected ErrForbidden for non-assignee marking in_progress, got %v", err)
	}
}

func TestTaskService_Update_Reviewer_CanMarkComplete_WhenActiveMember(t *testing.T) {
	appID := "app-1"
	termID := "term-1"
	activeStatus := models.ProgramMemberStatusActive
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "mentee-1", Status: "submitted", ApplicationID: &appID}, nil
		},
		update: func(_ context.Context, id string, _ models.TaskUpdateInput) (*models.Task, error) {
			return &models.Task{ID: id, Status: "complete", ApplicationID: &appID}, nil
		},
	}
	appRepo := &stubAppRepo{
		getByID: func(_ context.Context, _ string) (*models.Application, error) {
			return &models.Application{ID: appID, ProgramTermID: termID}, nil
		},
	}
	termRepo := &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: termID, ProgramID: "prog-1"}, nil
		},
	}
	memberRepo := &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: "mentor", Status: &activeStatus}, nil
		},
	}
	svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
	next := models.TaskStatusComplete
	_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{
		Status:  &next,
		ActorID: "mentor-1",
	})
	if err != nil {
		t.Errorf("active mentor should be able to mark complete: %v", err)
	}
}

func TestTaskService_Update_Reviewer_CannotMarkComplete_NotMember(t *testing.T) {
	appID := "app-1"
	termID := "term-1"
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "mentee-1", Status: "submitted", ApplicationID: &appID}, nil
		},
	}
	appRepo := &stubAppRepo{
		getByID: func(_ context.Context, _ string) (*models.Application, error) {
			return &models.Application{ID: appID, ProgramTermID: termID}, nil
		},
	}
	termRepo := &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: termID, ProgramID: "prog-1"}, nil
		},
	}
	// FindByProgramAndUser returns not-found → actor is not a member
	memberRepo := &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return nil, domain.ErrProgramMemberNotFound
		},
	}
	svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
	next := models.TaskStatusComplete
	_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{
		Status:  &next,
		ActorID: "stranger-99",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("expected ErrForbidden for non-member reviewer, got %v", err)
	}
}

func TestTaskService_Update_Reviewer_CannotMarkComplete_WrongRole(t *testing.T) {
	appID := "app-1"
	termID := "term-1"
	activeStatus := models.ProgramMemberStatusActive
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "mentee-1", Status: "submitted", ApplicationID: &appID}, nil
		},
	}
	appRepo := &stubAppRepo{
		getByID: func(_ context.Context, _ string) (*models.Application, error) {
			return &models.Application{ID: appID, ProgramTermID: termID}, nil
		},
	}
	termRepo := &stubTermRepo{
		getByID: func(_ context.Context, _ string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: termID, ProgramID: "prog-1"}, nil
		},
	}
	// Actor is a "mentee" member — not a mentor/program_admin
	memberRepo := &stubMemberRepo{
		findByProgramUser: func(_ context.Context, _, _ string) (*models.ProgramMember, error) {
			return &models.ProgramMember{MemberType: "mentee", Status: &activeStatus}, nil
		},
	}
	svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
	next := models.TaskStatusComplete
	_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{
		Status:  &next,
		ActorID: "mentee-2",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("expected ErrForbidden for member with wrong role, got %v", err)
	}
}

func TestTaskService_Update_InvalidTransition(t *testing.T) {
	appID := "app-1"
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "mentee-1", Status: "incomplete", ApplicationID: &appID}, nil
		},
	}
	svc := newTaskSvc(taskRepo, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{})
	next := models.TaskStatusComplete // incomplete → complete skips intermediate steps
	_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{
		Status:  &next,
		ActorID: "mentee-1",
	})
	// assignee cannot mark complete, so we'll get ErrForbidden before ErrInvalidStateTransition
	if !errors.Is(err, domain.ErrForbidden) && !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrForbidden or ErrInvalidStateTransition, got %v", err)
	}
}

func TestTaskService_Update_InvalidStatus_Rejected(t *testing.T) {
	svc := newTaskSvc(&stubTaskRepo{}, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{})
	bad := models.TaskStatus("flying") // not a member of the enum
	_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{Status: &bad})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for unknown status, got %v", err)
	}
}
