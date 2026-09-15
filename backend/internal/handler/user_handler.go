// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type userService interface {
	GetByID(ctx context.Context, id string) (*models.User, error)
}

// UserHandler holds Chi handlers for the users resource.
type UserHandler struct {
	svc userService
}

// NewUserHandler creates a UserHandler.
func NewUserHandler(svc userService) *UserHandler {
	return &UserHandler{svc: svc}
}

// GetByID handles GET /v1/users/{id}.
func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, user)
}
