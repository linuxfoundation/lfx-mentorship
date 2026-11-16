// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

// stubMentorInviteSvc records the token and caller each answer is made with,
// and fails every answer with err when it is set.
type stubMentorInviteSvc struct {
	token string
	actor models.UserPrincipal
	err   error
}

func (s *stubMentorInviteSvc) AcceptInvite(_ context.Context, token string, actor models.UserPrincipal) (*models.ProgramMember, error) {
	s.token, s.actor = token, actor
	if s.err != nil {
		return nil, s.err
	}
	return &models.ProgramMember{ID: "m-1"}, nil
}

func (s *stubMentorInviteSvc) DeclineInvite(_ context.Context, token string, actor models.UserPrincipal) error {
	s.token, s.actor = token, actor
	return s.err
}

// inviteAnswers are the two handlers that answer an invite.
var inviteAnswers = map[string]func(*handler.MentorInviteHandler, http.ResponseWriter, *http.Request){
	"accept":  (*handler.MentorInviteHandler).AcceptInvite,
	"decline": (*handler.MentorInviteHandler).DeclineInvite,
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
	wantCode := map[string]int{"accept": http.StatusOK, "decline": http.StatusNoContent}
	for name, answer := range inviteAnswers {
		svc := &stubMentorInviteSvc{}
		w := httptest.NewRecorder()
		answer(handler.NewMentorInviteHandler(svc), w, inviteRequest("tok", "user-1", "new-mentor"))
		if w.Code != wantCode[name] {
			t.Fatalf("%s: got %d; want %d", name, w.Code, wantCode[name])
		}
		if svc.token != "tok" || svc.actor != want {
			t.Errorf("%s: token = %q, actor = %+v; want %q, %+v", name, svc.token, svc.actor, "tok", want)
		}
	}
}

func TestMentorInviteHandler_RequiresAPrincipal(t *testing.T) {
	for name, answer := range inviteAnswers {
		svc := &stubMentorInviteSvc{}
		r := requestWithChiParam(httptest.NewRequest(http.MethodPost, "/v1/mentor-invites/tok/"+name, nil), "token", "tok")
		w := httptest.NewRecorder()
		answer(handler.NewMentorInviteHandler(svc), w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: got %d; want 401", name, w.Code)
		}
		if svc.token != "" {
			t.Errorf("%s: service called with token %q; want no call", name, svc.token)
		}
	}
}

func TestMentorInviteHandler_RequiresAToken(t *testing.T) {
	for name, answer := range inviteAnswers {
		svc := &stubMentorInviteSvc{}
		w := httptest.NewRecorder()
		answer(handler.NewMentorInviteHandler(svc), w, inviteRequest("", "user-1", "new-mentor"))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d; want 400", name, w.Code)
		}
		if svc.actor != (models.UserPrincipal{}) {
			t.Errorf("%s: service called by %+v; want no call", name, svc.actor)
		}
	}
}

// A refused answer reaches the caller as the status its error maps to.
func TestMentorInviteHandler_MapsServiceErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{domain.ErrInvalidInput, http.StatusBadRequest},
		{domain.ErrForbidden, http.StatusForbidden},
		{domain.ErrConflict, http.StatusConflict},
		{domain.ErrUpstreamUnavailable, http.StatusServiceUnavailable},
		{errors.New("db down"), http.StatusInternalServerError},
	} {
		for name, answer := range inviteAnswers {
			w := httptest.NewRecorder()
			answer(handler.NewMentorInviteHandler(&stubMentorInviteSvc{err: tc.err}), w, inviteRequest("tok", "user-1", "new-mentor"))
			if w.Code != tc.want {
				t.Errorf("%s with %v: got %d; want %d", name, tc.err, w.Code, tc.want)
			}
		}
	}
}
