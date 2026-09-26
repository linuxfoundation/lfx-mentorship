// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type userService interface {
	GetByID(ctx context.Context, id string) (*models.User, error)
	Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error)
	Delete(ctx context.Context, id, actorID string) error
	Bootstrap(ctx context.Context, lfid string, input models.UserUpdateInput) (*models.User, error)
}

// UserHandler holds Chi handlers for the signed-in user's own record.
type UserHandler struct {
	svc userService
}

// NewUserHandler creates a UserHandler.
func NewUserHandler(svc userService) *UserHandler {
	return &UserHandler{svc: svc}
}

// GetMe handles GET /v1/me — requires JWT.
func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	user, err := h.svc.GetByID(r.Context(), principal.UserID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, user)
}

// BootstrapMe handles PUT /v1/me and creates or refreshes the local user from the principal.
func (h *UserHandler) BootstrapMe(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var input models.UserUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	if input.Email == nil && principal.Email != "" {
		input.Email = &principal.Email
	}
	if input.Name == nil && principal.Name != "" {
		input.Name = &principal.Name
	}
	input.LFID = &principal.Username
	user, err := h.svc.Bootstrap(r.Context(), principal.Username, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, user)
}

// UpdateMe handles PATCH /v1/me — requires JWT.
func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	var input models.UserUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	// LFID is identity-bound and must not be mutable through update routes.
	input.LFID = nil

	user, err := h.svc.Update(r.Context(), principal.UserID, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, user)
}

// DeleteMe handles DELETE /v1/me — requires JWT.
func (h *UserHandler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	if err := h.svc.Delete(r.Context(), principal.UserID, principal.UserID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
