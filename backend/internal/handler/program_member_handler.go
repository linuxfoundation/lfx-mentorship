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

type programMemberService interface {
	GetByID(ctx context.Context, id string) (*models.ProgramMember, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error)
	Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error)
	Update(ctx context.Context, id string, input models.ProgramMemberUpdateInput) (*models.ProgramMember, error)
	Delete(ctx context.Context, id string) error
	ListAdminsByProgram(ctx context.Context, programID string) ([]*models.ProgramAdmin, error)
	AddAdmin(ctx context.Context, programID string, input models.ProgramAdminCreateInput) (*models.ProgramAdmin, error)
	DeleteAdmin(ctx context.Context, adminID string) error
}

// ProgramMemberHandler holds Chi handlers for program members and admins.
type ProgramMemberHandler struct {
	svc programMemberService
}

// NewProgramMemberHandler creates a ProgramMemberHandler.
func NewProgramMemberHandler(svc programMemberService) *ProgramMemberHandler {
	return &ProgramMemberHandler{svc: svc}
}

// List handles GET /v1/programs/{id}/members.
func (h *ProgramMemberHandler) List(w http.ResponseWriter, r *http.Request) {
	programID := chi.URLParam(r, "id")
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	members, meta, err := h.svc.ListByProgram(r.Context(), programID, models.ProgramMemberFilter{
		Limit:      limit,
		Offset:     offset,
		MemberType: r.URL.Query().Get("member_type"),
		Status:     r.URL.Query().Get("status"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": members, "meta": meta})
}

// Create handles POST /v1/programs/{id}/members — requires JWT.
func (h *ProgramMemberHandler) Create(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	programID := chi.URLParam(r, "id")
	var input models.ProgramMemberCreateInput
	if !decodeBody(w, r, &input) {
		return
	}

	member, err := h.svc.Create(r.Context(), programID, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, member)
}

// Update handles PATCH /v1/programs/{id}/members/{memberId} — requires JWT.
func (h *ProgramMemberHandler) Update(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	memberID := chi.URLParam(r, "memberId")
	var input models.ProgramMemberUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}

	member, err := h.svc.Update(r.Context(), memberID, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, member)
}

// Delete handles DELETE /v1/programs/{id}/members/{memberId} — requires JWT.
func (h *ProgramMemberHandler) Delete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	memberID := chi.URLParam(r, "memberId")
	if err := h.svc.Delete(r.Context(), memberID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListAdmins handles GET /v1/programs/{id}/admins.
func (h *ProgramMemberHandler) ListAdmins(w http.ResponseWriter, r *http.Request) {
	programID := chi.URLParam(r, "id")
	admins, err := h.svc.ListAdminsByProgram(r.Context(), programID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": admins})
}

// AddAdmin handles POST /v1/programs/{id}/admins — requires JWT.
func (h *ProgramMemberHandler) AddAdmin(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	programID := chi.URLParam(r, "id")
	var input models.ProgramAdminCreateInput
	if !decodeBody(w, r, &input) {
		return
	}

	admin, err := h.svc.AddAdmin(r.Context(), programID, input)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, admin)
}

// DeleteAdmin handles DELETE /v1/programs/{id}/admins/{adminId} — requires JWT.
func (h *ProgramMemberHandler) DeleteAdmin(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	adminID := chi.URLParam(r, "adminId")
	if err := h.svc.DeleteAdmin(r.Context(), adminID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
