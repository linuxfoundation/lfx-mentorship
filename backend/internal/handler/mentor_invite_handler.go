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

type mentorInviteService interface {
	GetInvite(ctx context.Context, token string, actor models.UserPrincipal) (*models.MentorInvite, error)
	AcceptInvite(ctx context.Context, token string, actor models.UserPrincipal) (*models.ProgramMember, error)
	DeclineInvite(ctx context.Context, token string, actor models.UserPrincipal) error
}

// MentorInviteHandler holds Chi handlers for reading and answering a mentor invite.
type MentorInviteHandler struct {
	svc mentorInviteService
}

// NewMentorInviteHandler creates a MentorInviteHandler.
func NewMentorInviteHandler(svc mentorInviteService) *MentorInviteHandler {
	return &MentorInviteHandler{svc: svc}
}

// GetInvite handles GET /v1/mentor-invites/{token} — requires JWT and invite token.
// It names the program so the invitee knows what they are answering.
func (h *MentorInviteHandler) GetInvite(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	token := chi.URLParam(r, "token")
	if token == "" {
		Error(w, newInvalidInput("token is required"))
		return
	}
	invite, err := h.svc.GetInvite(r.Context(), token, invitee(principal))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, invite)
}

// AcceptInvite handles POST /v1/mentor-invites/{token}/accept — requires JWT and invite token.
func (h *MentorInviteHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	token := chi.URLParam(r, "token")
	if token == "" {
		Error(w, newInvalidInput("token is required"))
		return
	}
	member, err := h.svc.AcceptInvite(r.Context(), token, invitee(principal))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, member)
}

// DeclineInvite handles POST /v1/mentor-invites/{token}/decline — requires JWT and invite token.
func (h *MentorInviteHandler) DeclineInvite(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	token := chi.URLParam(r, "token")
	if token == "" {
		Error(w, newInvalidInput("token is required"))
		return
	}
	if err := h.svc.DeclineInvite(r.Context(), token, invitee(principal)); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// invitee is the caller answering an invite: their user ID and their LFID, which
// an email invitation's address must belong to.
func invitee(principal *models.Principal) models.UserPrincipal {
	return models.UserPrincipal{UserID: principal.UserID, LFID: principal.Username}
}
