// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type mentorService interface {
	List(ctx context.Context, filter models.MentorFilter) (*models.MentorPage, error)
	Summary(ctx context.Context) (*models.MentorSummary, error)
	GetByUserID(ctx context.Context, userID string) (*models.MentorDetail, error)
}

// MentorHandler holds Chi handlers for the public mentor directory.
type MentorHandler struct {
	svc mentorService
}

// NewMentorHandler creates a MentorHandler.
func NewMentorHandler(svc mentorService) *MentorHandler {
	return &MentorHandler{svc: svc}
}

// List handles GET /v1/mentors.
func (h *MentorHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	page, err := h.svc.List(r.Context(), models.MentorFilter{
		Limit:  limit,
		Offset: offset,
		Search: r.URL.Query().Get("search"),
		Skill:  r.URL.Query().Get("skill"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, page)
}

// Summary handles GET /v1/mentors/summary.
func (h *MentorHandler) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.Summary(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, summary)
}

// GetByID handles GET /v1/mentors/{id}.
func (h *MentorHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := h.svc.GetByUserID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, detail)
}
