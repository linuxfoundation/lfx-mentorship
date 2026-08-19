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

type programTermService interface {
	GetByID(ctx context.Context, id string) (*models.ProgramTerm, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.ProgramTermCreateInput) (*models.ProgramTerm, error)
	Update(ctx context.Context, id string, input models.ProgramTermUpdateInput) (*models.ProgramTerm, error)
	Delete(ctx context.Context, id string) error
}

// ProgramTermHandler holds Chi handlers for the program terms resource.
type ProgramTermHandler struct {
	svc programTermService
}

// NewProgramTermHandler creates a ProgramTermHandler.
func NewProgramTermHandler(svc programTermService) *ProgramTermHandler {
	return &ProgramTermHandler{svc: svc}
}

// ListByProgram handles GET /v1/programs/{id}/terms.
func (h *ProgramTermHandler) ListByProgram(w http.ResponseWriter, r *http.Request) {
	programID := chi.URLParam(r, "id")
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	terms, meta, err := h.svc.ListByProgram(r.Context(), programID, models.ProgramTermFilter{
		Limit:     limit,
		Offset:    offset,
		ProgramID: programID,
		Status:    r.URL.Query().Get("status"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": terms, "meta": meta})
}

// GetByID handles GET /v1/program-terms/{id}.
func (h *ProgramTermHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	term, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, term)
}

// Create handles POST /v1/programs/{id}/terms — requires JWT.
func (h *ProgramTermHandler) Create(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	programID := chi.URLParam(r, "id")
	var input models.ProgramTermCreateInput
	if !decodeBody(w, r, &input) {
		return
	}
	input.ProgramID = programID

	term, err := h.svc.Create(r.Context(), input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, term)
}

// Update handles PATCH /v1/program-terms/{id} — requires JWT.
func (h *ProgramTermHandler) Update(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var input models.ProgramTermUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}

	term, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, term)
}

// Delete handles DELETE /v1/program-terms/{id} — requires JWT.
func (h *ProgramTermHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
