// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type userProfileService interface {
	GetByID(ctx context.Context, id string) (*models.UserProfile, error)
	GetBySlug(ctx context.Context, slug string) (*models.UserProfile, error)
	List(ctx context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, error)
	Update(ctx context.Context, id string, input models.UserProfileUpdateInput) (*models.UserProfile, error)
	Delete(ctx context.Context, id string) error
}

// UserProfileHandler holds Chi handlers for the user profiles resource.
type UserProfileHandler struct {
	svc userProfileService
}

// NewUserProfileHandler creates a UserProfileHandler.
func NewUserProfileHandler(svc userProfileService) *UserProfileHandler {
	return &UserProfileHandler{svc: svc}
}

// List handles GET /v1/user-profiles — paginated list with optional filters.
func (h *UserProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	filter := models.UserProfileFilter{
		Limit:       limit,
		Offset:      offset,
		UserID:      r.URL.Query().Get("user_id"),
		ProfileType: r.URL.Query().Get("profile_type"),
	}
	profiles, meta, err := h.svc.List(r.Context(), filter)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": profiles, "meta": meta})
}

// GetByID handles GET /v1/user-profiles/{id}.
func (h *UserProfileHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	profile, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, profile)
}

// GetBySlug handles GET /v1/user-profiles/slug/{slug}.
func (h *UserProfileHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	profile, err := h.svc.GetBySlug(r.Context(), slug)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, profile)
}

// Create handles POST /v1/user-profiles — requires JWT.
func (h *UserProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	var input models.UserProfileCreateInput
	if !decodeBody(w, r, &input) {
		return
	}

	profile, err := h.svc.Create(r.Context(), input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, profile)
}

// Update handles PATCH /v1/user-profiles/{id} — requires JWT.
func (h *UserProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var input models.UserProfileUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}

	profile, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, profile)
}

// Delete handles DELETE /v1/user-profiles/{id} — requires JWT.
func (h *UserProfileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
