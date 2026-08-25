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
	listByUser    func(ctx context.Context, userID string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	create        func(ctx context.Context, termID string, in models.ApplicationCreateInput) (*models.Application, error)
	getByID       func(ctx context.Context, id string) (*models.Application, error)
	listByTerm    func(ctx context.Context, termID string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	update        func(ctx context.Context, id string, in models.ApplicationUpdateInput) (*models.Application, error)
	delete        func(ctx context.Context, id string) error
	bulkDecline   func(ctx context.Context, termID string) (int, error)
	listPastMentees func(ctx context.Context, termID string) ([]*models.Application, error)
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
func (s *stubApplicationSvc) ListByProgramTerm(ctx context.Context, termID string, f models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
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
func (s *stubApplicationSvc) Delete(ctx context.Context, id string) error {
	if s.delete != nil {
		return s.delete(ctx, id)
	}
	return nil
}
func (s *stubApplicationSvc) BulkDeclineByTerm(ctx context.Context, termID string) (int, error) {
	if s.bulkDecline != nil {
		return s.bulkDecline(ctx, termID)
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
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func newApplicationHandler(svc *stubApplicationSvc) *handler.ApplicationHandler {
	return handler.NewApplicationHandler(svc)
}

// ── ListByUser ────────────────────────────────────────────────────────────────

func TestApplicationHandler_ListByUser_NoPrincipal_Returns401(t *testing.T) {
	h := newApplicationHandler(&stubApplicationSvc{})
	r := httptest.NewRequest(http.MethodGet, "/users/user-1/applications", nil)
	r = requestWithChiParam(r, "userId", "user-1")
	w := httptest.NewRecorder()
	h.ListByUser(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d; want 401", w.Code)
	}
}

func TestApplicationHandler_ListByUser_IDOR_Returns403(t *testing.T) {
	h := newApplicationHandler(&stubApplicationSvc{})
	r := httptest.NewRequest(http.MethodGet, "/users/other-user/applications", nil)
	r = requestWithPrincipal(r, "caller-user")
	r = requestWithChiParam(r, "userId", "other-user") // different from principal
	w := httptest.NewRecorder()
	h.ListByUser(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("got %d; want 403 (IDOR prevention)", w.Code)
	}
}

func TestApplicationHandler_ListByUser_OwnData_Returns200(t *testing.T) {
	h := newApplicationHandler(&stubApplicationSvc{})
	r := httptest.NewRequest(http.MethodGet, "/users/user-1/applications", nil)
	r = requestWithPrincipal(r, "user-1")
	r = requestWithChiParam(r, "userId", "user-1") // same as principal
	w := httptest.NewRecorder()
	h.ListByUser(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d; want 200", w.Code)
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

func TestApplicationHandler_Create_UserIDBoundToPrincipal(t *testing.T) {
	// Attacker tries to submit on behalf of "victim-user" by setting user_id in body.
	var capturedUserID string
	svc := &stubApplicationSvc{
		create: func(_ context.Context, _ string, in models.ApplicationCreateInput) (*models.Application, error) {
			capturedUserID = in.UserID
			return &models.Application{UserID: in.UserID, Role: in.Role, Status: "pending"}, nil
		},
	}
	h := newApplicationHandler(svc)

	body, _ := json.Marshal(map[string]string{
		"user_id": "victim-user", // attacker sets a foreign user_id
		"role":    "mentee",
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
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestApplicationHandler_GetByID_ServiceError_MapsToNotFound(t *testing.T) {
	svc := &stubApplicationSvc{
		getByID: func(_ context.Context, _ string) (*models.Application, error) {
			return nil, domain.ErrApplicationNotFound
		},
	}
	h := newApplicationHandler(svc)
	r := httptest.NewRequest(http.MethodGet, "/applications/missing", nil)
	r = requestWithChiParam(r, "id", "missing")
	w := httptest.NewRecorder()
	h.GetByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}
