// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"encoding/csv"
	"net/http"
	"strconv"

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
	BulkDeclineByTerm(ctx context.Context, termID string) (int, error)
	ListPastMenteesByTerm(ctx context.Context, termID string) ([]*models.Application, error)
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
	// Propagate caller identity for withdrawal guard.
	input.ActorID = principal.UserID

	app, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, app)
}

// Delete handles DELETE /v1/applications/{id} — requires JWT.
// Per FR-039, a mentee withdrawal sets status to "withdrawn" rather than deleting the record.
func (h *ApplicationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	withdrawn := "withdrawn"
	if _, err := h.svc.Update(r.Context(), id, models.ApplicationUpdateInput{
		Status:  &withdrawn,
		ActorID: principal.UserID,
	}); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BulkDeclineByTerm handles POST /v1/program-terms/{id}/applications/bulk-decline — requires JWT.
func (h *ApplicationHandler) BulkDeclineByTerm(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	termID := chi.URLParam(r, "id")
	count, err := h.svc.BulkDeclineByTerm(r.Context(), termID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"declined_count": count})
}

// ExportByTerm handles GET /v1/program-terms/{id}/applications/export — requires JWT.
// Returns a CSV of applications for the term, filterable by ?status=, per FR-045.
func (h *ApplicationHandler) ExportByTerm(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	termID := chi.URLParam(r, "id")
	q := r.URL.Query()
	filter := models.ApplicationFilter{
		Limit:  100_000, // export is unbounded
		Status: q.Get("status"),
		Role:   q.Get("role"),
	}
	if v := q.Get("tasks_submitted"); v == "true" {
		t := true
		filter.TasksSubmitted = &t
	} else if v == "false" {
		f := false
		filter.TasksSubmitted = &f
	}
	apps, _, err := h.svc.ListByProgramTerm(r.Context(), termID, filter)
	if err != nil {
		Error(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="applications.csv"`)
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "user_id", "role", "status", "attendance_type", "tasks_submitted", "created_on"})
	for _, a := range apps {
		attType := ""
		if a.AttendanceType != nil {
			attType = *a.AttendanceType
		}
		_ = cw.Write([]string{
			a.ID, a.UserID, a.Role, a.Status, attType,
			strconv.FormatBool(a.TasksSubmitted),
			strconv.FormatInt(a.CreatedOn.Unix(), 10),
		})
	}
	cw.Flush()
}

// PastMenteesByTerm handles GET /v1/program-terms/{id}/past-mentees — requires JWT.
func (h *ApplicationHandler) PastMenteesByTerm(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	termID := chi.URLParam(r, "id")
	apps, err := h.svc.ListPastMenteesByTerm(r.Context(), termID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": apps})
}
