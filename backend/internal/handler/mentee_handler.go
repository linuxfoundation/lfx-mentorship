// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type menteeService interface {
	List(ctx context.Context, filter models.MenteeFilter) (*models.MenteePage, error)
	Summary(ctx context.Context) (*models.MenteeSummary, error)
	GetByUserID(ctx context.Context, userID string) (*models.MenteeDetail, error)
}

// MenteeHandler holds Chi handlers for the public mentee directory.
type MenteeHandler struct {
	svc menteeService
}

// NewMenteeHandler creates a MenteeHandler.
func NewMenteeHandler(svc menteeService) *MenteeHandler {
	return &MenteeHandler{svc: svc}
}

// List handles GET /v1/mentees.
func (h *MenteeHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	page, err := h.svc.List(r.Context(), models.MenteeFilter{
		Limit:  limit,
		Offset: offset,
		Search: r.URL.Query().Get("search"),
		Skill:  r.URL.Query().Get("skill"),
		Status: r.URL.Query().Get("status"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, page)
}

// Summary handles GET /v1/mentees/summary.
func (h *MenteeHandler) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.Summary(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, summary)
}

// GetByID handles GET /v1/mentees/{id}.
func (h *MenteeHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := h.svc.GetByUserID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, detail)
}
