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
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type stubUserProfileSvc struct {
	list    func(context.Context, models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error)
	getByID func(context.Context, string) (*models.UserProfile, error)
	create  func(context.Context, models.UserProfileCreateInput) (*models.UserProfile, error)
	upsert  func(context.Context, models.UserProfileCreateInput) (*models.UserProfile, bool, error)
	update  func(context.Context, string, models.UserProfileUpdateInput) (*models.UserProfile, error)
	delete  func(context.Context, string) error
}

func (s *stubUserProfileSvc) GetByID(ctx context.Context, id string) (*models.UserProfile, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return &models.UserProfile{ID: id}, nil
}

func (s *stubUserProfileSvc) List(ctx context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error) {
	if s.list != nil {
		return s.list(ctx, filter)
	}
	return []*models.UserProfile{}, &models.PaginationMeta{}, nil
}

func (s *stubUserProfileSvc) Create(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, error) {
	if s.create != nil {
		return s.create(ctx, input)
	}
	return &models.UserProfile{ID: input.ID, UserID: input.UserID, ProfileType: input.ProfileType}, nil
}

func (s *stubUserProfileSvc) Upsert(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, bool, error) {
	if s.upsert != nil {
		return s.upsert(ctx, input)
	}
	return &models.UserProfile{ID: input.ID, UserID: input.UserID, ProfileType: input.ProfileType}, true, nil
}

func (s *stubUserProfileSvc) Update(ctx context.Context, id string, input models.UserProfileUpdateInput) (*models.UserProfile, error) {
	if s.update != nil {
		return s.update(ctx, id, input)
	}
	return &models.UserProfile{ID: id}, nil
}

func (s *stubUserProfileSvc) Delete(ctx context.Context, id string) error {
	if s.delete != nil {
		return s.delete(ctx, id)
	}
	return nil
}

func TestUserProfileHandler_ListMe_UsesPrincipalScope(t *testing.T) {
	var got models.UserProfileFilter
	h := handler.NewUserProfileHandler(&stubUserProfileSvc{
		list: func(_ context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error) {
			got = filter
			return []*models.UserProfile{{ID: "p1", UserID: "caller-user", ProfileType: "mentee"}}, &models.PaginationMeta{Total: 1}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/me/profiles?profile_type=mentee", nil)
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.ListMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if got.UserID != "caller-user" {
		t.Fatalf("got.UserID = %q; want caller-user", got.UserID)
	}
	if got.ProfileType != "mentee" {
		t.Fatalf("got.ProfileType = %q; want mentee", got.ProfileType)
	}
}

func TestUserProfileHandler_GetMeByType_RejectsMissingPrincipal(t *testing.T) {
	h := handler.NewUserProfileHandler(&stubUserProfileSvc{})
	r := httptest.NewRequest(http.MethodGet, "/v1/me/profiles/mentee", nil)
	w := httptest.NewRecorder()

	h.GetMeByType(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d; want 401", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] == nil {
		t.Fatal("expected error payload")
	}
}

func TestUserProfileHandler_GetMeByType_UsesPrincipalAndType(t *testing.T) {
	var got models.UserProfileFilter
	h := handler.NewUserProfileHandler(&stubUserProfileSvc{
		list: func(_ context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error) {
			got = filter
			return []*models.UserProfile{{ID: "p1", UserID: "caller-user", ProfileType: "mentee"}}, &models.PaginationMeta{Total: 1}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/me/profiles/mentee", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("profileType", "mentee")
	r = r.WithContext(context.WithValue(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetMeByType(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if got.UserID != "caller-user" {
		t.Fatalf("got.UserID = %q; want caller-user", got.UserID)
	}
	if got.ProfileType != "mentee" {
		t.Fatalf("got.ProfileType = %q; want mentee", got.ProfileType)
	}
	if got.Limit != 100 {
		t.Fatalf("got.Limit = %d; want 100 to detect duplicate profiles", got.Limit)
	}
	if got.Offset != 0 {
		t.Fatalf("got.Offset = %d; want 0", got.Offset)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] != "p1" {
		t.Fatalf("got id %#v; want p1", resp["id"])
	}
}

func TestUserProfileHandler_Create_BindsUserToPrincipal(t *testing.T) {
	var gotUserID string
	h := handler.NewUserProfileHandler(&stubUserProfileSvc{
		create: func(_ context.Context, input models.UserProfileCreateInput) (*models.UserProfile, error) {
			gotUserID = input.UserID
			return &models.UserProfile{ID: "p1", UserID: input.UserID, ProfileType: input.ProfileType}, nil
		},
	})
	body := []byte(`{"user_id":"victim","profile_type":"mentor"}`)
	r := httptest.NewRequest(http.MethodPost, "/v1/user-profiles", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}))
	w := httptest.NewRecorder()

	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("got %d; want 201", w.Code)
	}
	if gotUserID != "caller-user" {
		t.Fatalf("gotUserID = %q; want caller-user", gotUserID)
	}
}

func TestUserProfileHandler_PutMeByType_CreatesForPrincipal(t *testing.T) {
	var got models.UserProfileCreateInput
	h := handler.NewUserProfileHandler(&stubUserProfileSvc{
		list: func(context.Context, models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error) {
			return []*models.UserProfile{}, &models.PaginationMeta{}, nil
		},
		upsert: func(_ context.Context, input models.UserProfileCreateInput) (*models.UserProfile, bool, error) {
			got = input
			return &models.UserProfile{ID: "p1", UserID: input.UserID, ProfileType: input.ProfileType}, true, nil
		},
	})
	r := httptest.NewRequest(http.MethodPut, "/v1/me/profiles/mentee", bytes.NewBufferString(`{"user_id":"victim","profile_type":"mentor","age_eligible":true,"work_eligible":true}`))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("profileType", "mentee")
	r = r.WithContext(context.WithValue(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.PutMeByType(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("got %d; want 201", w.Code)
	}
	if got.UserID != "caller-user" || got.ProfileType != "mentee" {
		t.Fatalf("got owner/type %q/%q; want caller-user/mentee", got.UserID, got.ProfileType)
	}
}

func TestUserProfileHandler_UpdateMeByType_RejectsDuplicateProfiles(t *testing.T) {
	h := handler.NewUserProfileHandler(&stubUserProfileSvc{
		list: func(_ context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error) {
			if filter.UserID != "caller-user" {
				t.Fatalf("filter.UserID = %q; want caller-user", filter.UserID)
			}
			if filter.ProfileType != "mentee" {
				t.Fatalf("filter.ProfileType = %q; want mentee", filter.ProfileType)
			}
			return []*models.UserProfile{
				{ID: "p1", UserID: "caller-user", ProfileType: "mentee"},
				{ID: "p2", UserID: "caller-user", ProfileType: "mentee"},
			}, &models.PaginationMeta{Total: 2}, nil
		},
		update: func(context.Context, string, models.UserProfileUpdateInput) (*models.UserProfile, error) {
			t.Fatal("update should not be called when profile selection is ambiguous")
			return nil, nil
		},
	})

	r := httptest.NewRequest(http.MethodPatch, "/v1/me/profiles/mentee", bytes.NewBufferString(`{"about":"x"}`))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("profileType", "mentee")
	r = r.WithContext(context.WithValue(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateMeByType(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("got %d; want 409", w.Code)
	}
}

func TestUserProfileHandler_UpdateMeByID_UsesOwnedProfile(t *testing.T) {
	var updatedID string
	h := handler.NewUserProfileHandler(&stubUserProfileSvc{
		getByID: func(_ context.Context, id string) (*models.UserProfile, error) {
			return &models.UserProfile{ID: id, UserID: "caller-user", ProfileType: "mentor"}, nil
		},
		update: func(_ context.Context, id string, _ models.UserProfileUpdateInput) (*models.UserProfile, error) {
			updatedID = id
			return &models.UserProfile{ID: id, UserID: "caller-user"}, nil
		},
	})

	r := httptest.NewRequest(http.MethodPatch, "/v1/me/profiles/by-id/profile-2", bytes.NewBufferString(`{"about":"x"}`))
	r.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "profile-2")
	r = r.WithContext(context.WithValue(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "caller-user"}), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateMeByID(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if updatedID != "profile-2" {
		t.Fatalf("updated profile ID = %q; want profile-2", updatedID)
	}
}
