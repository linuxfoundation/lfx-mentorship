// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

// ── stub application service ─────────────────────────────────────────────────

type stubApplicationSvc struct {
	listByUser         func(ctx context.Context, userID string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	create             func(ctx context.Context, termID string, in models.ApplicationCreateInput) (*models.Application, error)
	getByID            func(ctx context.Context, id string) (*models.Application, error)
	getByIDForActor    func(ctx context.Context, id, actorID string) (*models.Application, error)
	listByTerm         func(ctx context.Context, termID string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	listByProgram      func(ctx context.Context, programID string, f models.ProgramApplicationFilter) ([]*models.ProgramApplicationRow, *models.PaginationMeta, error)
	listByTermForActor func(ctx context.Context, termID string, f models.ApplicationFilter, actorID string) ([]*models.Application, *models.PaginationMeta, error)
	update             func(ctx context.Context, id string, in models.ApplicationUpdateInput) (*models.Application, error)
	delete             func(ctx context.Context, id, actorID string) error
	bulkDecline        func(ctx context.Context, termID, actorID string) (int, error)
	listPastMentees    func(ctx context.Context, termID string) ([]*models.Application, error)
}

func (s *stubApplicationSvc) ListByUser(ctx context.Context, userID string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
	if s.listByUser != nil {
		return s.listByUser(ctx, userID, f)
	}
	return []*models.Application{}, &models.PaginationMeta{}, nil
}
func (s *stubApplicationSvc) Create(ctx context.Context, termID string, in models.ApplicationCreateInput) (*models.Application, error) {
	if s.create != nil {
		return s.create(ctx, termID, in)
	}
	return &models.Application{UserID: in.UserID}, nil
}
func (s *stubApplicationSvc) GetByID(ctx context.Context, id string) (*models.Application, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return &models.Application{ID: id}, nil
}
func (s *stubApplicationSvc) GetByIDForActor(ctx context.Context, id, actorID string) (*models.Application, error) {
	if s.getByIDForActor != nil {
		return s.getByIDForActor(ctx, id, actorID)
	}
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return &models.Application{ID: id, UserID: actorID}, nil
}
func (s *stubApplicationSvc) ListByProgramTerm(ctx context.Context, termID string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
	if s.listByTerm != nil {
		return s.listByTerm(ctx, termID, f)
	}
	return []*models.Application{}, &models.PaginationMeta{}, nil
}
func (s *stubApplicationSvc) ListByProgram(ctx context.Context, programID string, f models.ProgramApplicationFilter) ([]*models.ProgramApplicationRow, *models.PaginationMeta, error) {
	if s.listByProgram != nil {
		return s.listByProgram(ctx, programID, f)
	}
	return []*models.ProgramApplicationRow{}, &models.PaginationMeta{}, nil
}
func (s *stubApplicationSvc) ListByProgramTermForActor(ctx context.Context, termID string, f models.ApplicationFilter, actorID string) ([]*models.Application, *models.PaginationMeta, error) {
	if s.listByTermForActor != nil {
		return s.listByTermForActor(ctx, termID, f, actorID)
	}
	if s.listByTerm != nil {
		return s.listByTerm(ctx, termID, f)
	}
	return []*models.Application{}, &models.PaginationMeta{}, nil
}
func (s *stubApplicationSvc) Update(ctx context.Context, id string, in models.ApplicationUpdateInput) (*models.Application, error) {
	if s.update != nil {
		return s.update(ctx, id, in)
	}
	return &models.Application{ID: id}, nil
}
func (s *stubApplicationSvc) WithdrawForMentee(ctx context.Context, id, actorID string) (*models.Application, error) {
	return &models.Application{ID: id, UserID: actorID}, nil
}
func (s *stubApplicationSvc) WithdrawForMenteeAfterGatewayAuthorization(ctx context.Context, id string) (*models.Application, error) {
	return &models.Application{ID: id}, nil
}
func (s *stubApplicationSvc) Delete(ctx context.Context, id, actorID string) error {
	if s.delete != nil {
		return s.delete(ctx, id, actorID)
	}
	return nil
}
func (s *stubApplicationSvc) BulkDeclineByTerm(ctx context.Context, termID, actorID string) (int, error) {
	if s.bulkDecline != nil {
		return s.bulkDecline(ctx, termID, actorID)
	}
	return 0, nil
}
func (s *stubApplicationSvc) ListPastMenteesByTerm(ctx context.Context, termID string) ([]*models.Application, error) {
	if s.listPastMentees != nil {
		return s.listPastMentees(ctx, termID)
	}
	return nil, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// requestWithPrincipal attaches a principal to the request context.
func requestWithPrincipal(r *http.Request, userID string) *http.Request {
	p := &models.Principal{UserID: userID}
	return r.WithContext(auth.ContextWithPrincipal(r.Context(), p))
}

// requestWithChiParam attaches a chi route param to the request context.
func requestWithChiParam(r *http.Request, key, val string) *http.Request {
	rctx, _ := r.Context().Value(chi.RouteCtxKey).(*chi.Context)
	if rctx == nil {
		rctx = chi.NewRouteContext()
	}
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func newApplicationHandler(svc *stubApplicationSvc, termSvc ...*stubProgramTermSvc) *handler.ApplicationHandler {
	if len(termSvc) > 0 && termSvc[0] != nil {
		return handler.NewApplicationHandler(svc, termSvc[0])
	}
	return handler.NewApplicationHandler(svc)
}

// ── ListByMe ──────────────────────────────────────────────────────────────────

func TestApplicationHandler_ListByMe_NoPrincipal_Returns401(t *testing.T) {
	h := newApplicationHandler(&stubApplicationSvc{})
	r := httptest.NewRequest(http.MethodGet, "/me/applications", nil)
	w := httptest.NewRecorder()
	h.ListByMe(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d; want 401", w.Code)
	}
}

func TestApplicationHandler_ListByMe_UsesPrincipalAsScope(t *testing.T) {
	svc := &stubApplicationSvc{
		listByUser: func(_ context.Context, userID string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
			if userID != "caller-user" {
				t.Fatalf("userID = %q; want caller-user", userID)
			}
			return []*models.Application{{ID: "app-1"}}, &models.PaginationMeta{}, nil
		},
	}
	h := newApplicationHandler(svc)
	r := httptest.NewRequest(http.MethodGet, "/me/applications?status=pending", nil)
	r = requestWithPrincipal(r, "caller-user")
	w := httptest.NewRecorder()
	h.ListByMe(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestApplicationHandler_Create_NoPrincipal_Returns401(t *testing.T) {
	h := newApplicationHandler(&stubApplicationSvc{})
	body, _ := json.Marshal(map[string]string{"role": "mentee"})
	r := httptest.NewRequest(http.MethodPost, "/program-terms/term-1/applications", bytes.NewReader(body))
	r = requestWithChiParam(r, "id", "term-1")
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d; want 401", w.Code)
	}
}

func TestApplicationHandler_BulkDeclineByTerm_NoPrincipal_Returns401(t *testing.T) {
	h := newApplicationHandler(&stubApplicationSvc{})
	r := httptest.NewRequest(http.MethodPost, "/program-terms/term-1/applications/bulk-decline", nil)
	r = requestWithChiParam(r, "id", "term-1")
	w := httptest.NewRecorder()
	h.BulkDeclineByTerm(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d; want 401", w.Code)
	}
}

// The service repeats the gateway's program admin check off the gateway, so it needs the caller.
func TestApplicationHandler_AdminActionsPassPrincipalAsActor(t *testing.T) {
	var deleteActor, declineActor string
	h := newApplicationHandler(&stubApplicationSvc{
		delete: func(_ context.Context, _, actorID string) error {
			deleteActor = actorID
			return nil
		},
		bulkDecline: func(_ context.Context, _, actorID string) (int, error) {
			declineActor = actorID
			return 0, nil
		},
	})

	r := requestWithChiParam(requestWithPrincipal(httptest.NewRequest(http.MethodDelete, "/applications/app-1", nil), "admin-user"), "id", "app-1")
	w := httptest.NewRecorder()
	h.Delete(w, r)
	if w.Code != http.StatusNoContent || deleteActor != "admin-user" {
		t.Fatalf("delete: got %d with actor %q; want 204 with admin-user", w.Code, deleteActor)
	}

	r = requestWithChiParam(requestWithPrincipal(httptest.NewRequest(http.MethodPost, "/program-terms/term-1/applications/bulk-decline", nil), "admin-user"), "id", "term-1")
	w = httptest.NewRecorder()
	h.BulkDeclineByTerm(w, r)
	if w.Code != http.StatusOK || declineActor != "admin-user" {
		t.Fatalf("bulk decline: got %d with actor %q; want 200 with admin-user", w.Code, declineActor)
	}
}

func TestApplicationHandler_Create_UserIDBoundToPrincipal(t *testing.T) {
	// Attacker tries to submit on behalf of "victim-user" by setting user_id in body.
	var capturedUserID string
	var capturedAttendance *models.AttendanceType
	svc := &stubApplicationSvc{
		create: func(_ context.Context, _ string, in models.ApplicationCreateInput) (*models.Application, error) {
			capturedUserID = in.UserID
			capturedAttendance = in.AttendanceType
			return &models.Application{UserID: in.UserID, Role: in.Role, Status: "pending"}, nil
		},
	}
	h := newApplicationHandler(svc)

	body, _ := json.Marshal(map[string]string{
		"user_id":         "victim-user", // attacker sets a foreign user_id
		"role":            "mentee",
		"attendance_type": "full_time",
	})
	r := httptest.NewRequest(http.MethodPost, "/program-terms/term-1/applications", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = requestWithPrincipal(r, "caller-user") // authenticated as caller-user
	r = requestWithChiParam(r, "id", "term-1")

	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("got %d; want 201", w.Code)
	}
	// The principal's ID must always win, regardless of what the body said.
	if capturedUserID != "caller-user" {
		t.Errorf("service received UserID=%q; want %q (principal binding)", capturedUserID, "caller-user")
	}
	if capturedAttendance != nil {
		t.Errorf("service received AttendanceType=%q; want nil (admin sets it on acceptance)", *capturedAttendance)
	}
}

func TestApplicationHandler_Create_NestedRouteRejectsMismatchedProgramTerm(t *testing.T) {
	called := false
	svc := &stubApplicationSvc{
		create: func(_ context.Context, _ string, _ models.ApplicationCreateInput) (*models.Application, error) {
			called = true
			return &models.Application{}, nil
		},
	}
	h := newApplicationHandler(svc, &stubProgramTermSvc{
		getByProgramAndID: func(context.Context, string, string) (*models.ProgramTerm, error) {
			return nil, domain.ErrProgramTermNotFound
		},
	})
	body, _ := json.Marshal(map[string]string{"role": "mentee"})
	r := httptest.NewRequest(http.MethodPost, "/v1/programs/prog-1/terms/term-1/applications", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = requestWithPrincipal(r, "caller-user")
	r = requestWithChiParam(r, "programID", "prog-1")
	r = requestWithChiParam(r, "id", "term-1")
	w := httptest.NewRecorder()

	h.Create(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
	if called {
		t.Fatal("create should not be called when nested scope validation fails")
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestApplicationHandler_GetByID_ServiceError_MapsToNotFound(t *testing.T) {
	svc := &stubApplicationSvc{
		getByIDForActor: func(_ context.Context, _, _ string) (*models.Application, error) {
			return nil, domain.ErrApplicationNotFound
		},
	}
	h := newApplicationHandler(svc)
	r := httptest.NewRequest(http.MethodGet, "/applications/missing", nil)
	r = requestWithPrincipal(r, "u1")
	r = requestWithChiParam(r, "id", "missing")
	w := httptest.NewRecorder()
	h.GetByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}
