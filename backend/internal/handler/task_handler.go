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

type taskService interface {
	GetByID(ctx context.Context, id string) (*models.Task, error)
	ListByApplication(ctx context.Context, applicationID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	Create(ctx context.Context, applicationID string, input models.TaskCreateInput) (*models.Task, error)
	Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error)
	Delete(ctx context.Context, id string) error
}

// TaskHandler holds Chi handlers for tasks.
type TaskHandler struct {
	svc taskService
}

// NewTaskHandler creates a TaskHandler.
func NewTaskHandler(svc taskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

// ListByApplication handles GET /v1/applications/{id}/tasks.
func (h *TaskHandler) ListByApplication(w http.ResponseWriter, r *http.Request) {
	applicationID := chi.URLParam(r, "id")
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	tasks, meta, err := h.svc.ListByApplication(r.Context(), applicationID, models.TaskFilter{
		Limit:      limit,
		Offset:     offset,
		Status:     r.URL.Query().Get("status"),
		AssigneeID: r.URL.Query().Get("assignee_id"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": tasks, "meta": meta})
}

// ListByProgramTerm handles GET /v1/program-terms/{id}/tasks.
func (h *TaskHandler) ListByProgramTerm(w http.ResponseWriter, r *http.Request) {
	programTermID := chi.URLParam(r, "id")
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	tasks, meta, err := h.svc.ListByProgramTerm(r.Context(), programTermID, models.TaskFilter{
		Limit:      limit,
		Offset:     offset,
		Status:     r.URL.Query().Get("status"),
		AssigneeID: r.URL.Query().Get("assignee_id"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": tasks, "meta": meta})
}

// GetByID handles GET /v1/tasks/{id}.
func (h *TaskHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	task, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, task)
}

// Create handles POST /v1/applications/{id}/tasks — requires JWT.
func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	applicationID := chi.URLParam(r, "id")
	var input models.TaskCreateInput
	if !decodeBody(w, r, &input) {
		return
	}

	task, err := h.svc.Create(r.Context(), applicationID, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, task)
}

// Update handles PATCH /v1/tasks/{id} — requires JWT.
func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var input models.TaskUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	// Propagate caller identity for assignee permission check.
	input.ActorID = principal.UserID

	task, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, task)
}

// Delete handles DELETE /v1/tasks/{id} — requires JWT.
func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
