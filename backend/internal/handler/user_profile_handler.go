// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type userProfileService interface {
	GetByID(ctx context.Context, id string) (*models.UserProfile, error)
	List(ctx context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, error)
	Upsert(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, bool, error)
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

// ListMe handles GET /v1/me/profiles — requires JWT.
func (h *UserProfileHandler) ListMe(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	filter := models.UserProfileFilter{
		Limit:       limit,
		Offset:      offset,
		UserID:      principal.UserID,
		ProfileType: r.URL.Query().Get("profile_type"),
	}
	profiles, meta, err := h.svc.List(r.Context(), filter)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": profiles, "meta": meta})
}

// GetMeByType handles GET /v1/me/profiles/{profileType} — requires JWT.
func (h *UserProfileHandler) GetMeByType(w http.ResponseWriter, r *http.Request) {
	profile, ok := h.profileForPrincipal(w, r)
	if !ok {
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
	// Bind profile ownership to the authenticated principal.
	input.UserID = principal.UserID

	profile, err := h.svc.Create(r.Context(), input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, profile)
}

// PutMeByType creates or replaces the signed-in user's profile of the type in
// the path. It is the canonical registration endpoint for mentee profiles.
func (h *UserProfileHandler) PutMeByType(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	profileType := chi.URLParam(r, "profileType")
	if !models.UserProfileType(profileType).IsValid() {
		Error(w, fmt.Errorf("%w: profile_type must be mentor or mentee", domain.ErrInvalidInput))
		return
	}
	var input models.UserProfileCreateInput
	if !decodeBody(w, r, &input) {
		return
	}
	input.UserID = principal.UserID
	input.ProfileType = profileType
	profile, inserted, err := h.svc.Upsert(r.Context(), input)
	if err != nil {
		Error(w, err)
		return
	}
	status := http.StatusOK
	if inserted {
		status = http.StatusCreated
	}
	JSON(w, status, profile)
}

func (h *UserProfileHandler) profileForPrincipal(w http.ResponseWriter, r *http.Request) (*models.UserProfile, bool) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return nil, false
	}
	profileType := chi.URLParam(r, "profileType")
	profiles, _, err := h.svc.List(r.Context(), models.UserProfileFilter{
		Limit:       100,
		UserID:      principal.UserID,
		ProfileType: profileType,
	})
	if err != nil {
		Error(w, err)
		return nil, false
	}
	if len(profiles) > 1 {
		Error(w, fmt.Errorf("%w: multiple %s profiles exist for user", domain.ErrConflict, profileType))
		return nil, false
	}
	if len(profiles) == 0 {
		Error(w, domain.ErrUserProfileNotFound)
		return nil, false
	}
	return profiles[0], true
}

// UpdateMeByType handles PATCH /v1/me/profiles/{profileType}.
func (h *UserProfileHandler) UpdateMeByType(w http.ResponseWriter, r *http.Request) {
	profile, ok := h.profileForPrincipal(w, r)
	if !ok {
		return
	}
	var input models.UserProfileUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	updated, err := h.svc.Update(r.Context(), profile.ID, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, updated)
}

// DeleteMeByType handles DELETE /v1/me/profiles/{profileType}.
func (h *UserProfileHandler) DeleteMeByType(w http.ResponseWriter, r *http.Request) {
	profile, ok := h.profileForPrincipal(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), profile.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateMeByID handles PATCH /v1/me/profiles/by-id/{id}.
func (h *UserProfileHandler) UpdateMeByID(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	profile, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	if profile.UserID != principal.UserID {
		Error(w, domain.ErrForbidden)
		return
	}
	var input models.UserProfileUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	updated, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, updated)
}

// DeleteMeByID handles DELETE /v1/me/profiles/by-id/{id}.
func (h *UserProfileHandler) DeleteMeByID(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	profile, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	if profile.UserID != principal.UserID {
		Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
