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

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type stubUserSvc struct {
	getByID func(context.Context, string) (*models.User, error)
	update  func(context.Context, string, models.UserUpdateInput) (*models.User, error)
	delete  func(context.Context, string, string) error
}

func (s *stubUserSvc) GetByID(ctx context.Context, id string) (*models.User, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return &models.User{ID: id}, nil
}

func (s *stubUserSvc) Bootstrap(ctx context.Context, lfid string, input models.UserUpdateInput) (*models.User, error) {
	return &models.User{LFID: &lfid}, nil
}

func (s *stubUserSvc) Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
	if s.update != nil {
		return s.update(ctx, id, input)
	}
	return &models.User{ID: id}, nil
}

func (s *stubUserSvc) Delete(ctx context.Context, id, actorID string) error {
	if s.delete != nil {
		return s.delete(ctx, id, actorID)
	}
	return nil
}

func TestUserHandler_GetMe_UsesPrincipalAsActor(t *testing.T) {
	svc := &stubUserSvc{
		getByID: func(_ context.Context, id string) (*models.User, error) {
			if id != "caller-user" {
				t.Fatalf("id = %q; want caller-user", id)
			}
			return &models.User{ID: id, Name: ptrString("Caller")}, nil
		},
	}

	h := handler.NewUserHandler(svc)
	r := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.GetMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var body models.User
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != "caller-user" {
		t.Fatalf("body.ID = %q; want caller-user", body.ID)
	}
}

func TestUserHandler_UpdateMe_StoresPrincipalAsTarget(t *testing.T) {
	var gotID string
	var gotInput models.UserUpdateInput
	svc := &stubUserSvc{
		update: func(_ context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
			gotID = id
			gotInput = input
			return &models.User{ID: id}, nil
		},
	}

	h := handler.NewUserHandler(svc)
	body, _ := json.Marshal(map[string]string{"name": "Updated Name"})
	r := httptest.NewRequest(http.MethodPatch, "/v1/me", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.UpdateMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if gotID != "caller-user" {
		t.Fatalf("gotID = %q; want caller-user", gotID)
	}
	if gotInput.Name == nil || *gotInput.Name != "Updated Name" {
		t.Fatalf("gotInput.Name = %#v; want Updated Name", gotInput.Name)
	}
}

func TestUserHandler_DeleteMe_RequiresPrincipalAndUsesSelf(t *testing.T) {
	var gotActor string
	svc := &stubUserSvc{
		delete: func(_ context.Context, id, actorID string) error {
			if id != "caller-user" || actorID != "caller-user" {
				return domain.ErrForbidden
			}
			gotActor = actorID
			return nil
		},
	}

	h := handler.NewUserHandler(svc)
	r := httptest.NewRequest(http.MethodDelete, "/v1/me", nil)
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.DeleteMe(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("got %d; want 204", w.Code)
	}
	if gotActor != "caller-user" {
		t.Fatalf("gotActor = %q; want caller-user", gotActor)
	}
}

func TestUserHandler_UpdateMe_StripsLFIDFromPayload(t *testing.T) {
	var gotInput models.UserUpdateInput
	svc := &stubUserSvc{
		update: func(_ context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
			gotInput = input
			return &models.User{ID: id}, nil
		},
	}
	h := handler.NewUserHandler(svc)
	body, _ := json.Marshal(map[string]string{"lfid": "evil-overwrite", "name": "Allowed"})
	r := httptest.NewRequest(http.MethodPatch, "/v1/me", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.UpdateMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if gotInput.LFID != nil {
		t.Fatalf("gotInput.LFID = %v; want nil", gotInput.LFID)
	}
}

func ptrString(s string) *string { return &s }
