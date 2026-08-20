// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler

import (
	"context"
	"net/http"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type mentorInviteService interface {
	AcceptInvite(ctx context.Context, token string) (*models.ProgramMember, error)
	DeclineInvite(ctx context.Context, token string) error
}

// MentorInviteHandler holds Chi handlers for mentor invite accept/decline.
type MentorInviteHandler struct {
	svc mentorInviteService
}

// NewMentorInviteHandler creates a MentorInviteHandler.
func NewMentorInviteHandler(svc mentorInviteService) *MentorInviteHandler {
	return &MentorInviteHandler{svc: svc}
}

// AcceptInvite handles POST /v1/mentor-invites/accept — public (token is the credential).
func (h *MentorInviteHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Token == "" {
		Error(w, newInvalidInput("token is required"))
		return
	}
	member, err := h.svc.AcceptInvite(r.Context(), body.Token)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, member)
}

// DeclineInvite handles POST /v1/mentor-invites/decline — public (token is the credential).
func (h *MentorInviteHandler) DeclineInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Token == "" {
		Error(w, newInvalidInput("token is required"))
		return
	}
	if err := h.svc.DeclineInvite(r.Context(), body.Token); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
