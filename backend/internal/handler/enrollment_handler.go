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

type enrollmentService interface {
	GetByID(ctx context.Context, id string) (*models.Enrollment, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.EnrollmentFilter) ([]*models.Enrollment, *models.PaginationMeta, error)
	Create(ctx context.Context, programTermID string, input models.EnrollmentCreateInput) (*models.Enrollment, error)
	Update(ctx context.Context, id string, input models.EnrollmentUpdateInput) (*models.Enrollment, error)
}

// EnrollmentHandler holds Chi handlers for enrollments.
type EnrollmentHandler struct {
	svc enrollmentService
}

// NewEnrollmentHandler creates an EnrollmentHandler.
func NewEnrollmentHandler(svc enrollmentService) *EnrollmentHandler {
	return &EnrollmentHandler{svc: svc}
}

// ListByProgramTerm handles GET /v1/program-terms/{id}/enrollments.
func (h *EnrollmentHandler) ListByProgramTerm(w http.ResponseWriter, r *http.Request) {
	programTermID := chi.URLParam(r, "id")
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	enrollments, meta, err := h.svc.ListByProgramTerm(r.Context(), programTermID, models.EnrollmentFilter{
		Limit:  limit,
		Offset: offset,
		Status: r.URL.Query().Get("status"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": enrollments, "meta": meta})
}

// GetByID handles GET /v1/enrollments/{id}.
func (h *EnrollmentHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	enrollment, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, enrollment)
}

// Create handles POST /v1/program-terms/{id}/enrollments — requires JWT.
func (h *EnrollmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	programTermID := chi.URLParam(r, "id")
	var input models.EnrollmentCreateInput
	if !decodeBody(w, r, &input) {
		return
	}

	enrollment, err := h.svc.Create(r.Context(), programTermID, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, enrollment)
}

// Update handles PATCH /v1/enrollments/{id} — requires JWT.
func (h *EnrollmentHandler) Update(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var input models.EnrollmentUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}

	enrollment, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, enrollment)
}
