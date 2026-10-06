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
)

// stubTaskSvc implements the handler's task service; only Edit is exercised.
type stubTaskSvc struct {
	edit func(context.Context, string, models.TaskUpdateInput) (*models.Task, error)
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
func (s *stubTaskSvc) Create(context.Context, string, models.TaskCreateInput) (*models.Task, error) {
	return nil, nil
}
func (s *stubTaskSvc) Update(context.Context, string, models.TaskUpdateInput) (*models.Task, error) {
	return nil, nil
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
