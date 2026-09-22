// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type rosterStub struct {
	addProject func(context.Context, string, string, string) (*models.RosterMember, error)
}

func (s *rosterStub) ListApprovers(context.Context) ([]*models.RosterMember, error) {
	return []*models.RosterMember{}, nil
}
func (s *rosterStub) AddApprover(context.Context, string, string) (*models.RosterMember, error) {
	return &models.RosterMember{}, nil
}
func (s *rosterStub) RemoveApprover(context.Context, string, string) error { return nil }
func (s *rosterStub) ListProjectAdmins(context.Context, string) ([]*models.RosterMember, error) {
	return []*models.RosterMember{}, nil
}
func (s *rosterStub) AddProjectAdmin(ctx context.Context, actor, project, user string) (*models.RosterMember, error) {
	if s.addProject != nil {
		return s.addProject(ctx, actor, project, user)
	}
	return &models.RosterMember{}, nil
}
func (s *rosterStub) RemoveProjectAdmin(context.Context, string, string, string) error { return nil }

func rosterRequest(method, path string, scope string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "staff", Scope: scope}))
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("projectUID", "project-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	return r
}

func TestRosterHandler_ProjectAdminRequiresProjectScope(t *testing.T) {
	h := handler.NewRosterHandler(&rosterStub{})
	w := httptest.NewRecorder()
	h.ListProjectAdmins(w, rosterRequest(http.MethodGet, "/v1/projects/project-1/mentorship-program-admins", "manage:mentorship:project-admins:project-2"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d; want 403", w.Code)
	}
}

func TestRosterHandler_ProjectAdminUsesProjectScope(t *testing.T) {
	called := false
	h := handler.NewRosterHandler(&rosterStub{addProject: func(_ context.Context, actor, project, user string) (*models.RosterMember, error) {
		called = actor == "staff" && project == "project-1" && user == "user-2"
		return &models.RosterMember{}, nil
	}})
	r := httptest.NewRequest(http.MethodPost, "/v1/projects/project-1/mentorship-program-admins", strings.NewReader(`{"user_id":"user-2"}`))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "staff", Scope: "manage:mentorship:project-admins:project-1"}))
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("projectUID", "project-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()
	h.AddProjectAdmin(w, r)
	if w.Code == http.StatusForbidden {
		t.Fatalf("project-scoped management scope was rejected")
	}
	if !called {
		t.Fatal("service was not called with the scoped project")
	}
}
