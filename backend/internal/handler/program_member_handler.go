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
//
// This route is unauthenticated, so the member's email is stripped from every
// row before it is serialized. Email stays on the model because create and
// update accept it; it must not reach an anonymous caller.
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
	public := make([]models.ProgramMember, 0, len(members))
	for _, m := range members {
		redacted := *m
		redacted.Email = nil
		public = append(public, redacted)
	}
	JSON(w, http.StatusOK, map[string]any{"data": public, "meta": meta})
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
// Per FR-022, removing a mentor sets status to "withdrawn" rather than deleting the record.
func (h *ProgramMemberHandler) Delete(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}

	memberID := chi.URLParam(r, "memberId")
	withdrawn := models.ProgramMemberStatusWithdrawn
	if _, err := h.svc.Update(r.Context(), memberID, models.ProgramMemberUpdateInput{Status: &withdrawn}); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
