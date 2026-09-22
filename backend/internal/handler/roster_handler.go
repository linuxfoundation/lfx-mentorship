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

type rosterService interface {
	ListApprovers(context.Context) ([]*models.RosterMember, error)
	AddApprover(context.Context, string, string) (*models.RosterMember, error)
	RemoveApprover(context.Context, string, string) error
	ListProjectAdmins(context.Context, string) ([]*models.RosterMember, error)
	AddProjectAdmin(context.Context, string, string, string) (*models.RosterMember, error)
	RemoveProjectAdmin(context.Context, string, string, string) error
}

type RosterHandler struct{ svc rosterService }

func NewRosterHandler(svc rosterService) *RosterHandler { return &RosterHandler{svc: svc} }

func (h *RosterHandler) require(w http.ResponseWriter, r *http.Request, scope string) *models.Principal {
	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		Error(w, domain.ErrUnauthorized)
		return nil
	}
	if !auth.HasScope(r.Context(), scope) {
		Error(w, domain.ErrForbidden)
		return nil
	}
	return p
}

func (h *RosterHandler) ListApprovers(w http.ResponseWriter, r *http.Request) {
	if h.require(w, r, auth.ScopeManageApprovers()) == nil {
		return
	}
	items, err := h.svc.ListApprovers(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": items})
}
func (h *RosterHandler) AddApprover(w http.ResponseWriter, r *http.Request) {
	p := h.require(w, r, auth.ScopeManageApprovers())
	if p == nil {
		return
	}
	var input struct {
		UserID string `json:"user_id"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	item, err := h.svc.AddApprover(r.Context(), p.UserID, input.UserID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, item)
}
func (h *RosterHandler) RemoveApprover(w http.ResponseWriter, r *http.Request) {
	p := h.require(w, r, auth.ScopeManageApprovers())
	if p == nil {
		return
	}
	if err := h.svc.RemoveApprover(r.Context(), p.UserID, chi.URLParam(r, "userID")); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *RosterHandler) ListProjectAdmins(w http.ResponseWriter, r *http.Request) {
	if auth.PrincipalFromContext(r.Context()) == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if !auth.HasProjectManagementScope(r.Context(), chi.URLParam(r, "projectUID")) {
		Error(w, domain.ErrForbidden)
		return
	}
	items, err := h.svc.ListProjectAdmins(r.Context(), chi.URLParam(r, "projectUID"))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": items})
}
func (h *RosterHandler) AddProjectAdmin(w http.ResponseWriter, r *http.Request) {
	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if !auth.HasProjectManagementScope(r.Context(), chi.URLParam(r, "projectUID")) {
		Error(w, domain.ErrForbidden)
		return
	}
	var input struct {
		UserID string `json:"user_id"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	item, err := h.svc.AddProjectAdmin(r.Context(), p.UserID, chi.URLParam(r, "projectUID"), input.UserID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, item)
}
func (h *RosterHandler) RemoveProjectAdmin(w http.ResponseWriter, r *http.Request) {
	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if !auth.HasProjectManagementScope(r.Context(), chi.URLParam(r, "projectUID")) {
		Error(w, domain.ErrForbidden)
		return
	}
	if err := h.svc.RemoveProjectAdmin(r.Context(), p.UserID, chi.URLParam(r, "projectUID"), chi.URLParam(r, "userID")); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
