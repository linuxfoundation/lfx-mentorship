// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type taskService interface {
	GetByID(ctx context.Context, id string) (*models.Task, error)
	GetByIDForActor(ctx context.Context, id, actorID string) (*models.Task, error)
	ListByApplication(ctx context.Context, applicationID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	ListByApplicationForActor(ctx context.Context, applicationID string, filter models.TaskFilter, actorID string) ([]*models.Task, *models.PaginationMeta, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	ListByProgramTermForActor(ctx context.Context, programTermID string, filter models.TaskFilter, actorID string) ([]*models.Task, *models.PaginationMeta, error)
	Create(ctx context.Context, applicationID string, input models.TaskCreateInput) (*models.Task, error)
	Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error)
	Delete(ctx context.Context, id string, actorID string) error
}

type taskTermScopeService interface {
	GetByProgramAndID(ctx context.Context, programID, id string) (*models.ProgramTerm, error)
}

// TaskHandler holds Chi handlers for tasks.
type TaskHandler struct {
	svc     taskService
	termSvc taskTermScopeService
}

// NewTaskHandler creates a TaskHandler.
func NewTaskHandler(svc taskService, termSvc ...taskTermScopeService) *TaskHandler {
	h := &TaskHandler{svc: svc}
	if len(termSvc) > 0 {
		h.termSvc = termSvc[0]
	}
	return h
}

func (h *TaskHandler) validateNestedTermScope(ctx context.Context, r *http.Request, termID string) error {
	if h.termSvc == nil {
		return nil
	}
	programID := chi.URLParam(r, "programID")
	if programID == "" {
		programID = chi.URLParam(r, "program_uid")
	}
	if programID == "" {
		return nil
	}
	_, err := h.termSvc.GetByProgramAndID(ctx, programID, termID)
	return err
}

// ListByApplication handles GET /v1/applications/{id}/tasks.
func (h *TaskHandler) ListByApplication(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	applicationID := chi.URLParam(r, "id")
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	tasks, meta, err := h.svc.ListByApplicationForActor(r.Context(), applicationID, models.TaskFilter{
		Limit:      limit,
		Offset:     offset,
		Status:     r.URL.Query().Get("status"),
		AssigneeID: r.URL.Query().Get("assignee_id"),
	}, principal.UserID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": tasks, "meta": meta})
}

// ListByProgramTerm handles GET /v1/program-terms/{id}/tasks.
func (h *TaskHandler) ListByProgramTerm(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	programTermID := chi.URLParam(r, "id")
	if err := h.validateNestedTermScope(r.Context(), r, programTermID); err != nil {
		Error(w, err)
		return
	}
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	tasks, meta, err := h.svc.ListByProgramTermForActor(r.Context(), programTermID, models.TaskFilter{
		Limit:      limit,
		Offset:     offset,
		Status:     r.URL.Query().Get("status"),
		AssigneeID: r.URL.Query().Get("assignee_id"),
	}, principal.UserID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": tasks, "meta": meta})
}

// GetByID handles GET /v1/tasks/{id}.
func (h *TaskHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	task, err := h.svc.GetByIDForActor(r.Context(), id, principal.UserID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, task)
}

// UpdateSubmission handles PATCH /v1/tasks/{id}/submission.
func (h *TaskHandler) UpdateSubmission(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var input models.TaskUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	if input.Status == nil {
		Error(w, fmt.Errorf("%w: status is required", domain.ErrInvalidInput))
		return
	}
	input.ActorID = principal.UserID
	task, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), models.TaskUpdateInput{
		Status:  input.Status,
		File:    input.File,
		ActorID: input.ActorID,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, task)
}

// UpdateReview handles PATCH /v1/tasks/{id}/review.
func (h *TaskHandler) UpdateReview(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var input models.TaskUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	if input.Status == nil && input.ApplicationStatus == nil && input.ProgramTermStatus == nil {
		Error(w, fmt.Errorf("%w: review state is required", domain.ErrInvalidInput))
		return
	}
	input.ActorID = principal.UserID
	task, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), models.TaskUpdateInput{
		Status:            input.Status,
		ApplicationStatus: input.ApplicationStatus,
		ProgramTermStatus: input.ProgramTermStatus,
		ActorID:           input.ActorID,
	})
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
	if input.Status != nil || input.ApplicationStatus != nil || input.ProgramTermStatus != nil || input.SubmitFile != nil || input.File != nil {
		Error(w, fmt.Errorf("%w: task lifecycle states are handled by dedicated submission and review routes", domain.ErrInvalidInput))
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
	if err := h.svc.Delete(r.Context(), id, principal.UserID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
