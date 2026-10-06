// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

var allTaskStatuses = []models.TaskStatus{
	models.TaskStatusIncomplete, models.TaskStatusInProgress, models.TaskStatusSubmitted, models.TaskStatusComplete,
}

// reviewerRepos wires an application, term and program membership so actorID is an active
// reviewer of the program with the given member type.
func reviewerRepos(appID, actorID string, memberType models.MemberType) (*stubAppRepo, *stubTermRepo, *stubMemberRepo) {
	activeStatus := models.ProgramMemberStatusActive
	appRepo := &stubAppRepo{getByID: func(_ context.Context, _ string) (*models.Application, error) {
		return &models.Application{ID: appID, ProgramTermID: "term-1"}, nil
	}}
	termRepo := &stubTermRepo{getByID: func(_ context.Context, id string) (*models.ProgramTerm, error) {
		return &models.ProgramTerm{ID: id, ProgramID: "prog-1"}, nil
	}}
	memberRepo := &stubMemberRepo{findByProgramUser: func(_ context.Context, _, userID string) (*models.ProgramMember, error) {
		if userID != actorID {
			return nil, domain.ErrProgramMemberNotFound
		}
		return &models.ProgramMember{MemberType: memberType, Status: &activeStatus}, nil
	}}
	return appRepo, termRepo, memberRepo
}

func TestTaskService_Edit_ReviewerSetsAnyStatusFromAnyStatus(t *testing.T) {
	for _, memberType := range []models.MemberType{models.MemberTypeMentor, models.MemberTypeProgramAdmin} {
		for _, from := range allTaskStatuses {
			for _, to := range allTaskStatuses {
				t.Run(string(memberType)+"/"+string(from)+"->"+string(to), func(t *testing.T) {
					appID := "app-1"
					var written *models.TaskStatus
					taskRepo := &stubTaskRepo{
						getByID: func(_ context.Context, id string) (*models.Task, error) {
							return &models.Task{ID: id, AssigneeID: "mentee-1", Status: from, ApplicationID: &appID}, nil
						},
						update: func(_ context.Context, id string, in models.TaskUpdateInput) (*models.Task, error) {
							written = in.Status
							return &models.Task{ID: id, Status: *in.Status, ApplicationID: &appID}, nil
						},
					}
					appRepo, termRepo, memberRepo := reviewerRepos(appID, "reviewer-1", memberType)
					svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
					next := to
					task, err := svc.Edit(context.Background(), "task-1", models.TaskUpdateInput{Status: &next, ActorID: "reviewer-1"})
					if err != nil {
						t.Fatalf("Edit: %v", err)
					}
					if written == nil || *written != to || task.Status != to {
						t.Errorf("status written = %v, returned %q; want %q", written, task.Status, to)
					}
				})
			}
		}
	}
}

func TestTaskService_Edit_AssigneeForbiddenForEveryStatus(t *testing.T) {
	for _, from := range allTaskStatuses {
		for _, to := range allTaskStatuses {
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				appID := "app-1"
				taskRepo := &stubTaskRepo{
					getByID: func(_ context.Context, id string) (*models.Task, error) {
						return &models.Task{ID: id, AssigneeID: "mentor-1", Status: from, ApplicationID: &appID}, nil
					},
					update: func(context.Context, string, models.TaskUpdateInput) (*models.Task, error) {
						t.Fatal("assignee edit must not reach the repository")
						return nil, nil
					},
				}
				// The assignee is also an active mentor, and still may not edit their own task.
				appRepo, termRepo, memberRepo := reviewerRepos(appID, "mentor-1", models.MemberTypeMentor)
				svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
				next := to
				_, err := svc.Edit(context.Background(), "task-1", models.TaskUpdateInput{Status: &next, ActorID: "mentor-1"})
				if !errors.Is(err, domain.ErrForbidden) {
					t.Errorf("err = %v; want ErrForbidden", err)
				}
			})
		}
	}
}

func TestTaskService_Edit_AssigneeCannotChangeFileRequirement(t *testing.T) {
	appID := "app-1"
	taskRepo := &stubTaskRepo{getByID: func(_ context.Context, id string) (*models.Task, error) {
		return &models.Task{ID: id, AssigneeID: "mentee-1", Status: models.TaskStatusIncomplete, ApplicationID: &appID}, nil
	}}
	appRepo, termRepo, memberRepo := reviewerRepos(appID, "reviewer-1", models.MemberTypeMentor)
	svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
	required := "required"
	_, err := svc.Edit(context.Background(), "task-1", models.TaskUpdateInput{SubmitFile: &required, ActorID: "mentee-1"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v; want ErrForbidden", err)
	}
}

func TestTaskService_Edit_NonMemberForbiddenForMetadataOnly(t *testing.T) {
	appID := "app-1"
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "mentee-1", Status: models.TaskStatusIncomplete, ApplicationID: &appID}, nil
		},
		update: func(context.Context, string, models.TaskUpdateInput) (*models.Task, error) {
			t.Fatal("non-member edit must not reach the repository")
			return nil, nil
		},
	}
	appRepo, termRepo, memberRepo := reviewerRepos(appID, "reviewer-1", models.MemberTypeMentor)
	svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
	name, description, dueDate := "renamed", "new description", "2026-05-01"
	for field, in := range map[string]models.TaskUpdateInput{
		"name":        {Name: &name},
		"description": {Description: &description},
		"due date":    {DueDate: &dueDate},
	} {
		in.ActorID = "stranger-1"
		if _, err := svc.Edit(context.Background(), "task-1", in); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("%s: err = %v; want ErrForbidden", field, err)
		}
	}
}

func TestTaskService_Edit_MissingActorForbidden(t *testing.T) {
	svc := newTaskSvc(&stubTaskRepo{}, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{})
	name := "renamed"
	if _, err := svc.Edit(context.Background(), "task-1", models.TaskUpdateInput{Name: &name}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v; want ErrForbidden", err)
	}
}

func TestTaskService_Edit_TaskNotFound(t *testing.T) {
	taskRepo := &stubTaskRepo{getByID: func(context.Context, string) (*models.Task, error) {
		return nil, domain.ErrTaskNotFound
	}}
	svc := newTaskSvc(taskRepo, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{})
	name := "renamed"
	if _, err := svc.Edit(context.Background(), "task-1", models.TaskUpdateInput{Name: &name, ActorID: "reviewer-1"}); !errors.Is(err, domain.ErrTaskNotFound) {
		t.Errorf("err = %v; want ErrTaskNotFound", err)
	}
}

func TestTaskService_Edit_SetsAndClearsSubmitFileAndDueDate(t *testing.T) {
	required, dueDate, empty := "required", "2026-05-01", ""
	for name, in := range map[string]models.TaskUpdateInput{
		"set":   {SubmitFile: &required, DueDate: &dueDate},
		"clear": {SubmitFile: &empty, DueDate: &empty},
	} {
		t.Run(name, func(t *testing.T) {
			appID := "app-1"
			var got models.TaskUpdateInput
			taskRepo := &stubTaskRepo{
				getByID: func(_ context.Context, id string) (*models.Task, error) {
					return &models.Task{ID: id, AssigneeID: "mentee-1", ApplicationID: &appID}, nil
				},
				update: func(_ context.Context, id string, in models.TaskUpdateInput) (*models.Task, error) {
					got = in
					return &models.Task{ID: id}, nil
				},
			}
			appRepo, termRepo, memberRepo := reviewerRepos(appID, "reviewer-1", models.MemberTypeProgramAdmin)
			svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
			in.ActorID = "reviewer-1"
			if _, err := svc.Edit(context.Background(), "task-1", in); err != nil {
				t.Fatalf("Edit: %v", err)
			}
			if got.SubmitFile != in.SubmitFile || got.DueDate != in.DueDate {
				t.Errorf("repository got submit_file %v, due_date %v; want them passed through", got.SubmitFile, got.DueDate)
			}
		})
	}
}

func TestTaskService_Edit_RejectsInvalidInput(t *testing.T) {
	badStatus, badCategory, badDate, file := models.TaskStatus("flying"), models.TaskCategory("optional"), "05/01/2026", "key"
	svc := newTaskSvc(&stubTaskRepo{}, &stubAppRepo{}, &stubTermRepo{}, &stubMemberRepo{})
	for field, in := range map[string]models.TaskUpdateInput{
		"status":   {Status: &badStatus},
		"category": {Category: &badCategory},
		"due date": {DueDate: &badDate},
		"file":     {File: &file},
	} {
		in.ActorID = "reviewer-1"
		if _, err := svc.Edit(context.Background(), "task-1", in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: err = %v; want ErrInvalidInput", field, err)
		}
	}
}

func TestTaskService_Edit_StatusOrCategoryChecksTasksSubmitted(t *testing.T) {
	status, category, name := models.TaskStatusIncomplete, models.TaskCategoryNonPrerequisite, "renamed"
	for label, tc := range map[string]struct {
		in        models.TaskUpdateInput
		wantCheck bool
	}{
		// Leaving submitted keeps tasks_submitted set; the repository only ever sets the flag.
		"status":   {in: models.TaskUpdateInput{Status: &status}, wantCheck: true},
		"category": {in: models.TaskUpdateInput{Category: &category}, wantCheck: true},
		"name":     {in: models.TaskUpdateInput{Name: &name}, wantCheck: false},
	} {
		t.Run(label, func(t *testing.T) {
			appID := "app-1"
			checked := false
			taskRepo := &stubTaskRepo{
				getByID: func(_ context.Context, id string) (*models.Task, error) {
					return &models.Task{ID: id, AssigneeID: "mentee-1", Status: models.TaskStatusSubmitted, ApplicationID: &appID}, nil
				},
				update: func(_ context.Context, id string, _ models.TaskUpdateInput) (*models.Task, error) {
					return &models.Task{ID: id, ApplicationID: &appID}, nil
				},
			}
			appRepo, termRepo, memberRepo := reviewerRepos(appID, "reviewer-1", models.MemberTypeMentor)
			appRepo.markTasksSubmit = func(context.Context, string) (bool, error) {
				checked = true
				return false, nil
			}
			svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
			tc.in.ActorID = "reviewer-1"
			if _, err := svc.Edit(context.Background(), "task-1", tc.in); err != nil {
				t.Fatalf("Edit: %v", err)
			}
			if checked != tc.wantCheck {
				t.Errorf("tasks_submitted checked = %v; want %v", checked, tc.wantCheck)
			}
		})
	}
}

func TestTaskService_Edit_RepositoryErrorPropagates(t *testing.T) {
	appID := "app-1"
	taskRepo := &stubTaskRepo{
		getByID: func(_ context.Context, id string) (*models.Task, error) {
			return &models.Task{ID: id, AssigneeID: "mentee-1", ApplicationID: &appID}, nil
		},
		update: func(context.Context, string, models.TaskUpdateInput) (*models.Task, error) {
			return nil, domain.ErrStateLocked
		},
	}
	appRepo, termRepo, memberRepo := reviewerRepos(appID, "reviewer-1", models.MemberTypeMentor)
	svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
	name := "renamed"
	if _, err := svc.Edit(context.Background(), "task-1", models.TaskUpdateInput{Name: &name, ActorID: "reviewer-1"}); !errors.Is(err, domain.ErrStateLocked) {
		t.Errorf("err = %v; want ErrStateLocked", err)
	}
}

func TestTaskService_Update_ReviewWithoutStatusRequiresNonAssigneeReviewer(t *testing.T) {
	appID := "app-1"
	accepted := models.ApplicationStatusAccepted
	for actor, wantErr := range map[string]error{
		"reviewer-1": nil,
		"stranger-1": domain.ErrForbidden,
		"mentee-1":   domain.ErrForbidden,
	} {
		t.Run(actor, func(t *testing.T) {
			taskRepo := &stubTaskRepo{getByID: func(_ context.Context, id string) (*models.Task, error) {
				return &models.Task{ID: id, AssigneeID: "mentee-1", Status: models.TaskStatusSubmitted, ApplicationID: &appID}, nil
			}}
			appRepo, termRepo, memberRepo := reviewerRepos(appID, "reviewer-1", models.MemberTypeMentor)
			svc := newTaskSvc(taskRepo, appRepo, termRepo, memberRepo)
			_, err := svc.Update(context.Background(), "task-1", models.TaskUpdateInput{ApplicationStatus: &accepted, ActorID: actor})
			if !errors.Is(err, wantErr) {
				t.Errorf("err = %v; want %v", err, wantErr)
			}
		})
	}
}
