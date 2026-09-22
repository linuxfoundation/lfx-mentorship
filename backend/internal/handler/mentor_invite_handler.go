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
	AcceptInvite(ctx context.Context, token, actorID string) (*models.ProgramMember, error)
	DeclineInvite(ctx context.Context, token, actorID string) error
}

// MentorInviteHandler holds Chi handlers for mentor invite accept/decline.
type MentorInviteHandler struct {
	svc mentorInviteService
}

// NewMentorInviteHandler creates a MentorInviteHandler.
func NewMentorInviteHandler(svc mentorInviteService) *MentorInviteHandler {
	return &MentorInviteHandler{svc: svc}
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
	member, err := h.svc.AcceptInvite(r.Context(), token, principal.UserID)
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
	if err := h.svc.DeclineInvite(r.Context(), token, principal.UserID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
