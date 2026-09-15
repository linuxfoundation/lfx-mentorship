// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type userProfileService interface {
	GetByID(ctx context.Context, id string) (*models.UserProfile, error)
	GetBySlug(ctx context.Context, slug string) (*models.UserProfile, error)
}

// UserProfileHandler holds Chi handlers for the user profiles resource.
type UserProfileHandler struct {
	svc userProfileService
}

// NewUserProfileHandler creates a UserProfileHandler.
func NewUserProfileHandler(svc userProfileService) *UserProfileHandler {
	return &UserProfileHandler{svc: svc}
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
