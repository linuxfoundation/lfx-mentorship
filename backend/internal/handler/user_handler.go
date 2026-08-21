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

type userService interface {
	GetByID(ctx context.Context, id string) (*models.User, error)
	List(ctx context.Context, filter models.UserFilter) ([]*models.User, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.UserCreateInput) (*models.User, error)
	Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error)
	Delete(ctx context.Context, id string) error
}

// UserHandler holds Chi handlers for the users resource.
type UserHandler struct {
	svc userService
}

// NewUserHandler creates a UserHandler.
func NewUserHandler(svc userService) *UserHandler {
	return &UserHandler{svc: svc}
}

// List handles GET /v1/users — paginated list of users.
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	filter := models.UserFilter{
		Limit:  limit,
		Offset: offset,
		Search: r.URL.Query().Get("search"),
	}
	users, meta, err := h.svc.List(r.Context(), filter)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": users, "meta": meta})
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

// Create handles POST /v1/users — requires JWT.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	var input models.UserCreateInput
	if !decodeBody(w, r, &input) {
		return
	}

	user, err := h.svc.Create(r.Context(), input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, user)
}

// Update handles PATCH /v1/users/{id} — requires JWT.
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var input models.UserUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}

	user, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, user)
}

// Delete handles DELETE /v1/users/{id} — requires JWT.
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
