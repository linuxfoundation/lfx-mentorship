// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

// stubMentorInviteSvc records the token and caller each answer is made with.
type stubMentorInviteSvc struct {
	token string
	actor models.UserPrincipal
}

func (s *stubMentorInviteSvc) AcceptInvite(_ context.Context, token string, actor models.UserPrincipal) (*models.ProgramMember, error) {
	s.token, s.actor = token, actor
	return &models.ProgramMember{ID: "m-1"}, nil
}

func (s *stubMentorInviteSvc) DeclineInvite(_ context.Context, token string, actor models.UserPrincipal) error {
	s.token, s.actor = token, actor
	return nil
}

// inviteRequest is a request for token from the principal userID / username.
func inviteRequest(token, userID, username string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/mentor-invites/"+token+"/accept", nil)
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: userID, Username: username}))
	return requestWithChiParam(r, "token", token)
}

// An email invitation's address must belong to the caller's LFID, so the
// handler must pass the principal's user ID and username through as UserID and
// LFID on both answers.
func TestMentorInviteHandler_PassesTheCallerAsInvitee(t *testing.T) {
	want := models.UserPrincipal{UserID: "user-1", LFID: "new-mentor"}
	for name, answer := range map[string]struct {
		call     func(*handler.MentorInviteHandler, http.ResponseWriter, *http.Request)
		wantCode int
	}{
		"accept":  {(*handler.MentorInviteHandler).AcceptInvite, http.StatusOK},
		"decline": {(*handler.MentorInviteHandler).DeclineInvite, http.StatusNoContent},
	} {
		svc := &stubMentorInviteSvc{}
		w := httptest.NewRecorder()
		answer.call(handler.NewMentorInviteHandler(svc), w, inviteRequest("tok", "user-1", "new-mentor"))
		if w.Code != answer.wantCode {
			t.Fatalf("%s: got %d; want %d", name, w.Code, answer.wantCode)
		}
		if svc.token != "tok" || svc.actor != want {
			t.Errorf("%s: token = %q, actor = %+v; want %q, %+v", name, svc.token, svc.actor, "tok", want)
		}
	}
}

func TestMentorInviteHandler_RequiresAPrincipal(t *testing.T) {
	svc := &stubMentorInviteSvc{}
	r := requestWithChiParam(httptest.NewRequest(http.MethodPost, "/v1/mentor-invites/tok/accept", nil), "token", "tok")
	w := httptest.NewRecorder()
	handler.NewMentorInviteHandler(svc).AcceptInvite(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d; want 401", w.Code)
	}
	if svc.token != "" {
		t.Errorf("service called with token %q; want no call", svc.token)
	}
}
