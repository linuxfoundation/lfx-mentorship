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
	ListMentorManagement(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMentorManagementRow, *models.PaginationMeta, error)
	Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error)
	Update(ctx context.Context, programID, id string, input models.ProgramMemberUpdateInput, actorID string) (*models.ProgramMember, error)
	Delete(ctx context.Context, programID, id, actorID string) error
	ListMine(ctx context.Context, userID string, filter models.ProgramMemberFilter) ([]*models.ProgramMembership, *models.PaginationMeta, error)
	RequestMentorship(ctx context.Context, programID, userID string) (*models.ProgramMember, error)
	WithdrawMine(ctx context.Context, id, userID string) error
}

func (h *ProgramMemberHandler) ListMentorManagement(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	rows, meta, err := h.svc.ListMentorManagement(r.Context(), chi.URLParam(r, "id"), models.ProgramMemberFilter{Limit: limit, Offset: offset, Status: r.URL.Query().Get("status"), Search: r.URL.Query().Get("search")})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": rows, "meta": meta})
}

// ProgramMemberHandler holds Chi handlers for program members and admins.
type ProgramMemberHandler struct {
	svc        programMemberService
	programSvc programLookup
}

// NewProgramMemberHandler creates a ProgramMemberHandler. programSvc is used
// only to resolve the program behind the {id} path parameter and apply the
// hidden-program rule to the public roster.
func NewProgramMemberHandler(svc programMemberService, programSvc programLookup) *ProgramMemberHandler {
	return &ProgramMemberHandler{svc: svc, programSvc: programSvc}
}

// List handles GET /v1/programs/{id}/members.
//
// This route is unauthenticated, so it serves the public roster only: active
// members, with the email stripped from every row before it is serialized.
//
// The program is resolved through resolveVisibleProgram so a hidden program is
// a 404 here exactly as it is on the program itself (FR-009) — otherwise a
// caller holding a hidden program's ID could enumerate its admins and mentors.
//
// Status is pinned rather than read from the query string — a caller must not
// be able to widen a public roster to pending, declined or withdrawn members.
// Email stays on the model because create and update accept it; it must not
// reach an anonymous caller.
func (h *ProgramMemberHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	program, ok := resolveVisibleProgram(w, r, h.programSvc)
	if !ok {
		return
	}
	if program.Status != models.ProgramStatusPublished {
		Error(w, domain.ErrProgramNotFound)
		return
	}
	members, meta, err := h.svc.ListByProgram(r.Context(), program.ID, models.ProgramMemberFilter{
		Limit:      limit,
		Offset:     offset,
		MemberType: string(models.MemberTypeMentor),
		Status:     string(models.ProgramMemberStatusApproved),
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

	programID := chi.URLParam(r, "id")
	memberID := chi.URLParam(r, "memberId")

	var input models.ProgramMemberUpdateInput
	if !decodeBody(w, r, &input) {
		return
	}

	member, err := h.svc.Update(r.Context(), programID, memberID, input, principal.UserID)
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

	programID := chi.URLParam(r, "id")
	memberID := chi.URLParam(r, "memberId")

	withdrawn := models.ProgramMemberStatusWithdrawn
	if _, err := h.svc.Update(r.Context(), programID, memberID, models.ProgramMemberUpdateInput{Status: &withdrawn}, principal.UserID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListMine handles GET /v1/me/program-memberships — the caller's own rows in
// any status, each with its program's name.
func (h *ProgramMemberHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	limit, offset, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}
	memberships, meta, err := h.svc.ListMine(r.Context(), principal.UserID, models.ProgramMemberFilter{
		Limit:      limit,
		Offset:     offset,
		MemberType: r.URL.Query().Get("member_type"),
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": memberships, "meta": meta})
}

// RequestMine handles POST /v1/me/program-memberships — the caller's own
// request to mentor a program. The user is the principal, never the body.
func (h *ProgramMemberHandler) RequestMine(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var input models.ProgramMembershipRequestInput
	if !decodeBody(w, r, &input) {
		return
	}
	member, err := h.svc.RequestMentorship(r.Context(), input.ProgramID, principal.UserID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, member)
}

// WithdrawMine handles POST /v1/me/program-memberships/{id}/withdraw.
func (h *ProgramMemberHandler) WithdrawMine(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if err := h.svc.WithdrawMine(r.Context(), chi.URLParam(r, "id"), principal.UserID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
