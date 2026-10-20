// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

// stubTaskSvc implements the handler's task service; only Create, Edit and Update are exercised.
type stubTaskSvc struct {
	create func(context.Context, string, models.TaskCreateInput) (*models.Task, error)
	edit   func(context.Context, string, models.TaskUpdateInput) (*models.Task, error)
	update func(context.Context, string, models.TaskUpdateInput) (*models.Task, error)
}

func (s *stubTaskSvc) GetByID(context.Context, string) (*models.Task, error) { return nil, nil }
func (s *stubTaskSvc) GetByIDForActor(context.Context, string, string) (*models.Task, error) {
	return nil, nil
}
func (s *stubTaskSvc) ListByApplication(context.Context, string, models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	return nil, nil, nil
}
func (s *stubTaskSvc) ListByApplicationForActor(context.Context, string, models.TaskFilter, string) ([]*models.Task, *models.PaginationMeta, error) {
	return nil, nil, nil
}
func (s *stubTaskSvc) ListByProgramTerm(context.Context, string, models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	return nil, nil, nil
}
func (s *stubTaskSvc) ListByProgramTermForActor(context.Context, string, models.TaskFilter, string) ([]*models.Task, *models.PaginationMeta, error) {
	return nil, nil, nil
}
func (s *stubTaskSvc) Create(ctx context.Context, applicationID string, in models.TaskCreateInput) (*models.Task, error) {
	return s.create(ctx, applicationID, in)
}
func (s *stubTaskSvc) Update(ctx context.Context, id string, in models.TaskUpdateInput) (*models.Task, error) {
	return s.update(ctx, id, in)
}
func (s *stubTaskSvc) Edit(ctx context.Context, id string, in models.TaskUpdateInput) (*models.Task, error) {
	return s.edit(ctx, id, in)
}
func (s *stubTaskSvc) Delete(context.Context, string, string) error { return nil }

func patchTask(t *testing.T, svc *stubTaskSvc, body string, withPrincipal bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPatch, "/v1/tasks/task-1", strings.NewReader(body))
	r = requestWithChiParam(r, "id", "task-1")
	if withPrincipal {
		r = requestWithPrincipal(r, "reviewer-1")
	}
	w := httptest.NewRecorder()
	handler.NewTaskHandler(svc).Update(w, r)
	return w
}

func TestTaskHandler_Update_ForwardsFullEditToService(t *testing.T) {
	var gotID string
	var got models.TaskUpdateInput
	svc := &stubTaskSvc{edit: func(_ context.Context, id string, in models.TaskUpdateInput) (*models.Task, error) {
		gotID, got = id, in
		return &models.Task{ID: id, Status: *in.Status}, nil
	}}
	w := patchTask(t, svc, `{"name":"n","status":"complete","submit_file":"","due_date":""}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s; want 200", w.Code, w.Body)
	}
	if gotID != "task-1" || got.ActorID != "reviewer-1" || got.Status == nil || *got.Status != models.TaskStatusComplete {
		t.Errorf("service got id %q, input %+v", gotID, got)
	}
	if got.SubmitFile == nil || *got.SubmitFile != "" || got.DueDate == nil || *got.DueDate != "" {
		t.Errorf("empty submit_file and due_date must reach the service as clears: %+v", got)
	}
	if !strings.Contains(w.Body.String(), `"status":"complete"`) {
		t.Errorf("body = %s; want the updated task", w.Body)
	}
}

func TestTaskHandler_Update_RejectsReviewAndFileFields(t *testing.T) {
	svc := &stubTaskSvc{edit: func(context.Context, string, models.TaskUpdateInput) (*models.Task, error) {
		t.Fatal("rejected body must not reach the service")
		return nil, nil
	}}
	for _, body := range []string{
		`{"application_status":"accepted"}`,
		`{"program_term_status":"open"}`,
		`{"file":"key"}`,
	} {
		if w := patchTask(t, svc, body, true); w.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d; want 400", body, w.Code)
		}
	}
}

func TestTaskHandler_UpdateReview_AcceptsOnlyCompleteOrIncomplete(t *testing.T) {
	for status, want := range map[string]int{
		"complete":    http.StatusOK,
		"incomplete":  http.StatusOK,
		"in_progress": http.StatusBadRequest,
		"submitted":   http.StatusBadRequest,
	} {
		svc := &stubTaskSvc{update: func(_ context.Context, id string, in models.TaskUpdateInput) (*models.Task, error) {
			return &models.Task{ID: id, Status: *in.Status}, nil
		}}
		r := httptest.NewRequest(http.MethodPatch, "/v1/tasks/task-1/review", strings.NewReader(`{"status":"`+status+`"}`))
		r = requestWithPrincipal(requestWithChiParam(r, "id", "task-1"), "reviewer-1")
		w := httptest.NewRecorder()
		handler.NewTaskHandler(svc).UpdateReview(w, r)
		if w.Code != want {
			t.Errorf("%s: code = %d, body %s; want %d", status, w.Code, w.Body, want)
		}
	}
}

func TestTaskHandler_Create_BindsCreatorAndOwnerToPrincipal(t *testing.T) {
	for _, tc := range []struct {
		name      string
		principal *models.Principal
		wantOwner *string
	}{
		{"user", &models.Principal{UserID: "caller-1", Username: "caller"}, ptrString("caller-1")},
		{"m2m", &models.Principal{UserID: "svc-1", Username: "svc@clients"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got models.TaskCreateInput
			svc := &stubTaskSvc{create: func(_ context.Context, _ string, in models.TaskCreateInput) (*models.Task, error) {
				got = in
				return &models.Task{ID: "task-1"}, nil
			}}
			r := httptest.NewRequest(http.MethodPost, "/v1/applications/app-1/tasks",
				strings.NewReader(`{"assignee_id":"mentee-1","created_by":"someone-else","owner_id":"other-user"}`))
			r = requestWithChiParam(r, "id", "app-1")
			r = r.WithContext(auth.ContextWithPrincipal(r.Context(), tc.principal))
			w := httptest.NewRecorder()
			handler.NewTaskHandler(svc).Create(w, r)
			if w.Code != http.StatusCreated {
				t.Fatalf("code = %d, body %s; want 201", w.Code, w.Body)
			}
			if got.CreatedBy == nil || *got.CreatedBy != tc.principal.UserID {
				t.Errorf("created_by = %v; want %q", got.CreatedBy, tc.principal.UserID)
			}
			if (got.OwnerID == nil) != (tc.wantOwner == nil) || (got.OwnerID != nil && *got.OwnerID != *tc.wantOwner) {
				t.Errorf("owner_id = %v; want %v", got.OwnerID, tc.wantOwner)
			}
		})
	}
}

func TestTaskHandler_Update_RequiresPrincipal(t *testing.T) {
	svc := &stubTaskSvc{edit: func(context.Context, string, models.TaskUpdateInput) (*models.Task, error) {
		t.Fatal("unauthenticated request must not reach the service")
		return nil, nil
	}}
	if w := patchTask(t, svc, `{"name":"n"}`, false); w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d; want 401", w.Code)
	}
}

func TestTaskHandler_Update_MapsServiceErrors(t *testing.T) {
	for err, want := range map[error]int{
		domain.ErrInvalidInput: http.StatusBadRequest,
		domain.ErrForbidden:    http.StatusForbidden,
		domain.ErrTaskNotFound: http.StatusNotFound,
		domain.ErrStateLocked:  http.StatusConflict,
	} {
		svc := &stubTaskSvc{edit: func(context.Context, string, models.TaskUpdateInput) (*models.Task, error) {
			return nil, err
		}}
		if w := patchTask(t, svc, `{"name":"n"}`, true); w.Code != want {
			t.Errorf("%v: code = %d; want %d", err, w.Code, want)
		}
	}
}
