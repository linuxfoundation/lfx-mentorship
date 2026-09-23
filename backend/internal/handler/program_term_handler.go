// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type programTermService interface {
	GetByID(ctx context.Context, id string) (*models.ProgramTerm, error)
	GetByProgramAndID(ctx context.Context, programID, id string) (*models.ProgramTerm, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error)
	ListManagementByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTermManagementRow, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.ProgramTermCreateInput) (*models.ProgramTerm, error)
	Update(ctx context.Context, id string, input models.ProgramTermUpdateInput) (*models.ProgramTerm, error)
	Delete(ctx context.Context, id string) error
	Close(ctx context.Context, id string) (*models.ProgramTerm, int, error)
	Reopen(ctx context.Context, id string) (*models.ProgramTerm, error)
}

// ProgramTermHandler holds Chi handlers for the program terms resource.
type ProgramTermHandler struct {
	svc programTermService
}

// NewProgramTermHandler creates a ProgramTermHandler.
func NewProgramTermHandler(svc programTermService) *ProgramTermHandler {
	return &ProgramTermHandler{svc: svc}
}

// termWithLabel wraps a ProgramTerm with a computed discovery_label.
type termWithLabel struct {
	*models.ProgramTerm
	DiscoveryLabel string `json:"discovery_label"`
}

func withLabel(t *models.ProgramTerm) termWithLabel {
	return termWithLabel{ProgramTerm: t, DiscoveryLabel: t.DiscoveryLabel(time.Now())}
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
	labeled := make([]termWithLabel, len(terms))
	for i, t := range terms {
		labeled[i] = withLabel(t)
	}
	JSON(w, http.StatusOK, map[string]any{"data": labeled, "meta": meta})
}

func (h *ProgramTermHandler) ListManagementByProgram(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	rows, meta, err := h.svc.ListManagementByProgram(r.Context(), chi.URLParam(r, "id"), models.ProgramTermFilter{Limit: limit, Offset: offset})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": rows, "meta": meta})
}

// GetByID handles GET /v1/programs/{programID}/terms/{termID} and legacy /v1/program-terms/{id}.
func (h *ProgramTermHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	programID := chi.URLParam(r, "programID")
	if programID == "" {
		programID = chi.URLParam(r, "program_uid")
	}
	id := chi.URLParam(r, "termID")
	if id == "" {
		id = chi.URLParam(r, "id")
	}

	var (
		term *models.ProgramTerm
		err  error
	)
	if programID != "" {
		term, err = h.svc.GetByProgramAndID(r.Context(), programID, id)
	} else {
		term, err = h.svc.GetByID(r.Context(), id)
	}
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, withLabel(term))
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

// Update handles PATCH /v1/programs/{programID}/terms/{termID} and legacy /v1/program-terms/{id} — requires JWT.
func (h *ProgramTermHandler) Update(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	programID := chi.URLParam(r, "programID")
	if programID == "" {
		programID = chi.URLParam(r, "program_uid")
	}
	id := chi.URLParam(r, "termID")
	if id == "" {
		id = chi.URLParam(r, "id")
	}
	var input models.ProgramTermUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}

	if programID != "" {
		if _, err := h.svc.GetByProgramAndID(r.Context(), programID, id); err != nil {
			Error(w, err)
			return
		}
	}

	term, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, term)
}

// Delete handles DELETE /v1/programs/{programID}/terms/{termID} and legacy /v1/program-terms/{id} — requires JWT.
func (h *ProgramTermHandler) Delete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	programID := chi.URLParam(r, "programID")
	if programID == "" {
		programID = chi.URLParam(r, "program_uid")
	}
	id := chi.URLParam(r, "termID")
	if id == "" {
		id = chi.URLParam(r, "id")
	}
	if programID != "" {
		if _, err := h.svc.GetByProgramAndID(r.Context(), programID, id); err != nil {
			Error(w, err)
			return
		}
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Close handles POST /v1/programs/{programID}/terms/{termID}/close.
func (h *ProgramTermHandler) Close(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	programID, termID := chi.URLParam(r, "programID"), chi.URLParam(r, "termID")
	if _, err := h.svc.GetByProgramAndID(r.Context(), programID, termID); err != nil {
		Error(w, err)
		return
	}
	term, _, err := h.svc.Close(r.Context(), termID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, term)
}

// Reopen handles POST /v1/programs/{programID}/terms/{termID}/reopen.
func (h *ProgramTermHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	programID, termID := chi.URLParam(r, "programID"), chi.URLParam(r, "termID")
	if _, err := h.svc.GetByProgramAndID(r.Context(), programID, termID); err != nil {
		Error(w, err)
		return
	}
	term, err := h.svc.Reopen(r.Context(), termID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, term)
}
