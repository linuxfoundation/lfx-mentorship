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

type applicationService interface {
	GetByID(ctx context.Context, id string) (*models.Application, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	ListByUser(ctx context.Context, userID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	Create(ctx context.Context, programTermID string, input models.ApplicationCreateInput) (*models.Application, error)
	Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error)
	Delete(ctx context.Context, id string) error
}

// ApplicationHandler holds Chi handlers for applications.
type ApplicationHandler struct {
	svc applicationService
}

// NewApplicationHandler creates an ApplicationHandler.
func NewApplicationHandler(svc applicationService) *ApplicationHandler {
	return &ApplicationHandler{svc: svc}
}

// ListByProgramTerm handles GET /v1/program-terms/{id}/applications.
func (h *ApplicationHandler) ListByProgramTerm(w http.ResponseWriter, r *http.Request) {
	programTermID := chi.URLParam(r, "id")
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	apps, meta, err := h.svc.ListByProgramTerm(r.Context(), programTermID, models.ApplicationFilter{
		Limit:  limit,
		Offset: offset,
		Status: r.URL.Query().Get("status"),
		Role:   r.URL.Query().Get("role"),
		UserID: r.URL.Query().Get("user_id"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": apps, "meta": meta})
}

// ListByUser handles GET /v1/users/{userId}/applications — requires JWT.
func (h *ApplicationHandler) ListByUser(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	userID := chi.URLParam(r, "userId")
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	apps, meta, err := h.svc.ListByUser(r.Context(), userID, models.ApplicationFilter{
		Limit:  limit,
		Offset: offset,
		Status: r.URL.Query().Get("status"),
		Role:   r.URL.Query().Get("role"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": apps, "meta": meta})
}

// GetByID handles GET /v1/applications/{id}.
func (h *ApplicationHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, app)
}

// Create handles POST /v1/program-terms/{id}/applications — requires JWT.
func (h *ApplicationHandler) Create(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	programTermID := chi.URLParam(r, "id")
	var input models.ApplicationCreateInput
	if !decodeBody(w, r, &input) {
		return
	}

	app, err := h.svc.Create(r.Context(), programTermID, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, app)
}

// Update handles PATCH /v1/applications/{id} — requires JWT.
func (h *ApplicationHandler) Update(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var input models.ApplicationUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}

	app, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, app)
}

// Delete handles DELETE /v1/applications/{id} — requires JWT.
func (h *ApplicationHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
