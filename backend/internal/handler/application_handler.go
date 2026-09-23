// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type applicationService interface {
	GetByID(ctx context.Context, id string) (*models.Application, error)
	GetByIDForActor(ctx context.Context, id, actorID string) (*models.Application, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramApplicationFilter) ([]*models.ProgramApplicationRow, *models.PaginationMeta, error)
	ListByProgramTermForActor(ctx context.Context, programTermID string, filter models.ApplicationFilter, actorID string) ([]*models.Application, *models.PaginationMeta, error)
	ListByUser(ctx context.Context, userID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	Create(ctx context.Context, programTermID string, input models.ApplicationCreateInput) (*models.Application, error)
	Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error)
	Delete(ctx context.Context, id string) error
	WithdrawForMentee(ctx context.Context, id, actorID string) (*models.Application, error)
	WithdrawForMenteeAfterGatewayAuthorization(ctx context.Context, id string) (*models.Application, error)
	BulkDeclineByTerm(ctx context.Context, termID string) (int, error)
	ListPastMenteesByTerm(ctx context.Context, termID string) ([]*models.Application, error)
}

func (h *ApplicationHandler) ListByProgram(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	rows, meta, err := h.svc.ListByProgram(r.Context(), chi.URLParam(r, "id"), models.ProgramApplicationFilter{Limit: limit, Offset: offset, Type: models.ProgramApplicationType(r.URL.Query().Get("type")), Search: r.URL.Query().Get("search"), Status: r.URL.Query().Get("status"), TermID: r.URL.Query().Get("term")})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": rows, "meta": meta})
}

type applicationTermScopeService interface {
	GetByProgramAndID(ctx context.Context, programID, id string) (*models.ProgramTerm, error)
}

// ApplicationHandler holds Chi handlers for applications.
type ApplicationHandler struct {
	svc     applicationService
	termSvc applicationTermScopeService
}

// NewApplicationHandler creates an ApplicationHandler.
func NewApplicationHandler(svc applicationService, termSvc ...applicationTermScopeService) *ApplicationHandler {
	h := &ApplicationHandler{svc: svc}
	if len(termSvc) > 0 {
		h.termSvc = termSvc[0]
	}
	return h
}

func (h *ApplicationHandler) validateNestedTermScope(ctx context.Context, r *http.Request, termID string) error {
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

// ListByProgramTerm handles GET /v1/program-terms/{id}/applications.
func (h *ApplicationHandler) ListByProgramTerm(w http.ResponseWriter, r *http.Request) {
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
	apps, meta, err := h.svc.ListByProgramTermForActor(r.Context(), programTermID, models.ApplicationFilter{
		Limit:  limit,
		Offset: offset,
		Status: r.URL.Query().Get("status"),
		Role:   r.URL.Query().Get("role"),
		UserID: r.URL.Query().Get("user_id"),
	}, principal.UserID)
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
	// Principle VII-1: reject IDOR — callers may only list their own applications.
	if userID != principal.UserID {
		Error(w, domain.ErrForbidden)
		return
	}
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

// ListByMe handles GET /v1/me/applications — requires JWT.
func (h *ApplicationHandler) ListByMe(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	apps, meta, err := h.svc.ListByUser(r.Context(), principal.UserID, models.ApplicationFilter{
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
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	app, err := h.svc.GetByIDForActor(r.Context(), id, principal.UserID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, app)
}

// UpdateStatus handles PATCH /v1/applications/{id}/status.
func (h *ApplicationHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var input models.ApplicationUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	if input.Status == nil {
		Error(w, fmt.Errorf("%w: status is required", domain.ErrInvalidInput))
		return
	}
	input.ActorID = principal.UserID
	app, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), models.ApplicationUpdateInput{
		Status:         input.Status,
		AttendanceType: input.AttendanceType,
		ActorID:        input.ActorID,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, app)
}

// UpdateEvaluation handles PUT /v1/applications/{id}/evaluation.
func (h *ApplicationHandler) UpdateEvaluation(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var input models.ApplicationUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	if input.Evaluation == nil || input.Status != nil || input.ReviewerNote != nil {
		Error(w, fmt.Errorf("%w: evaluation is required and cannot update status or reviewer note", domain.ErrInvalidInput))
		return
	}
	app, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), models.ApplicationUpdateInput{
		Evaluation: input.Evaluation,
		ActorID:    auth.PrincipalFromContext(r.Context()).UserID,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"evaluation": app.Evaluation})
}

// GetNote handles GET /v1/applications/{id}/note.
func (h *ApplicationHandler) GetNote(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	app, err := h.svc.GetByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"note": app.ReviewerNote})
}

// UpdateNote handles PUT /v1/applications/{id}/note.
func (h *ApplicationHandler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var input models.ApplicationUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}
	if input.ReviewerNote == nil || input.Status != nil || input.Evaluation != nil {
		Error(w, fmt.Errorf("%w: reviewer note is required and cannot update status or evaluation", domain.ErrInvalidInput))
		return
	}
	app, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), models.ApplicationUpdateInput{
		ReviewerNote: input.ReviewerNote,
		ActorID:      auth.PrincipalFromContext(r.Context()).UserID,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"note": app.ReviewerNote})
}

// Withdraw handles POST /v1/applications/{id}/withdraw.
func (h *ApplicationHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	status := models.ApplicationStatusWithdrawn
	app, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), models.ApplicationUpdateInput{Status: &status, ActorID: principal.UserID})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, app)
}

// WithdrawForMentee handles manager-authorized assisted withdrawal.
func (h *ApplicationHandler) WithdrawForMentee(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var app *models.Application
	var err error
	if auth.IsGatewayPrincipal(r.Context()) {
		app, err = h.svc.WithdrawForMenteeAfterGatewayAuthorization(r.Context(), chi.URLParam(r, "id"))
	} else {
		app, err = h.svc.WithdrawForMentee(r.Context(), chi.URLParam(r, "id"), principal.UserID)
	}
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, app)
}

// Reapply handles POST /v1/applications/{id}/reapply.
func (h *ApplicationHandler) Reapply(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	app, err := h.svc.GetByIDForActor(r.Context(), chi.URLParam(r, "id"), principal.UserID)
	if err != nil {
		Error(w, err)
		return
	}
	if app.UserID != principal.UserID {
		Error(w, domain.ErrForbidden)
		return
	}
	replacement, err := h.svc.Create(r.Context(), app.ProgramTermID, models.ApplicationCreateInput{
		UserID: principal.UserID,
		Role:   app.Role,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, replacement)
}

// Create handles POST /v1/program-terms/{id}/applications — requires JWT.
func (h *ApplicationHandler) Create(w http.ResponseWriter, r *http.Request) {
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
	var input models.ApplicationCreateInput
	if !decodeBody(w, r, &input) {
		return
	}
	// Principle VII-2: bind ownership to the authenticated principal, not the request body.
	input.UserID = principal.UserID

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
	if input.Status != nil || input.TasksSubmitted != nil || input.AdminNotified != nil || input.Evaluation != nil || input.ReviewerNote != nil {
		Error(w, fmt.Errorf("%w: protected application fields are handled by dedicated routes", domain.ErrInvalidInput))
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

// BulkDeclineByTerm handles POST /v1/program-terms/{id}/applications/bulk-decline — requires JWT.
func (h *ApplicationHandler) BulkDeclineByTerm(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	termID := chi.URLParam(r, "id")
	if err := h.validateNestedTermScope(r.Context(), r, termID); err != nil {
		Error(w, err)
		return
	}
	count, err := h.svc.BulkDeclineByTerm(r.Context(), termID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"declined": count})
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
	if err := h.validateNestedTermScope(r.Context(), r, termID); err != nil {
		Error(w, err)
		return
	}
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
			attType = string(*a.AttendanceType)
		}
		_ = cw.Write([]string{
			a.ID, a.UserID, string(a.Role), string(a.Status), attType,
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
	if err := h.validateNestedTermScope(r.Context(), r, termID); err != nil {
		Error(w, err)
		return
	}
	apps, err := h.svc.ListPastMenteesByTerm(r.Context(), termID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": apps})
}
