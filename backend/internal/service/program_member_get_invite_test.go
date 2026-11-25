// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

const invitedProgramName = "Open Source Mentorship"

// namedProgRepo holds every program under invitedProgramName, at status.
func namedProgRepo(status models.ProgramStatus) *stubProgRepo {
	return &stubProgRepo{getByID: func(_ context.Context, id string) (*models.Program, error) {
		return &models.Program{ID: id, Name: invitedProgramName, Status: status}, nil
	}}
}

// userInviteRepo holds mentor-1's mentor row on prog-1 at status, or none when status is nil.
func userInviteRepo(status *models.ProgramMemberStatus) *stubMemberRepo {
	return &stubMemberRepo{
		findByUserAndType: func(_ context.Context, programID, userID string, _ models.MemberType) (*models.ProgramMember, error) {
			if status == nil || programID != "prog-1" || userID != "mentor-1" {
				return nil, domain.ErrProgramMemberNotFound
			}
			return &models.ProgramMember{ID: "member-1", ProgramID: programID, UserID: userID, Status: status}, nil
		},
	}
}

func userInviteToken(t *testing.T) string {
	t.Helper()
	token, err := auth.GenerateInviteToken("prog-1", "mentor-1", "test-secret")
	if err != nil {
		t.Fatalf("GenerateInviteToken: %v", err)
	}
	return token
}

var wantInvite = models.MentorInvite{ProgramID: "prog-1", ProgramName: invitedProgramName}

func TestProgramMemberService_GetInvite_UserInvite(t *testing.T) {
	// A program that is not published is still named: the read only says what the invite is for.
	svc := newMemberSvc(userInviteRepo(memberStatus(models.ProgramMemberStatusInvited)), namedProgRepo(models.ProgramStatusPending), &stubNotifier{})
	got, err := svc.GetInvite(context.Background(), userInviteToken(t), models.UserPrincipal{UserID: "mentor-1"})
	if err != nil {
		t.Fatalf("GetInvite: %v", err)
	}
	if *got != wantInvite {
		t.Errorf("got %+v; want %+v", *got, wantInvite)
	}
}

func TestProgramMemberService_GetInvite_UserInviteErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		token  string
		status *models.ProgramMemberStatus
		caller string
		want   error
	}{
		"malformed token":     {token: "not-a-valid-token", status: memberStatus(models.ProgramMemberStatusInvited), caller: "mentor-1", want: domain.ErrInvalidInput},
		"another user's":      {status: memberStatus(models.ProgramMemberStatusInvited), caller: "different-user", want: domain.ErrForbidden},
		"no caller":           {status: memberStatus(models.ProgramMemberStatusInvited), want: domain.ErrForbidden},
		"already accepted":    {status: memberStatus(models.ProgramMemberStatusApproved), caller: "mentor-1", want: domain.ErrInvalidInput},
		"declined or revoked": {status: memberStatus(models.ProgramMemberStatusDeclined), caller: "mentor-1", want: domain.ErrInvalidInput},
		"row removed":         {caller: "mentor-1", want: domain.ErrInvalidInput},
	} {
		token := tc.token
		if token == "" {
			token = userInviteToken(t)
		}
		svc := newMemberSvc(userInviteRepo(tc.status), namedProgRepo(models.ProgramStatusPublished), &stubNotifier{})
		if got, err := svc.GetInvite(context.Background(), token, models.UserPrincipal{UserID: tc.caller}); !errors.Is(err, tc.want) || got != nil {
			t.Errorf("%s: got %+v, err = %v; want %v", name, got, err, tc.want)
		}
	}
}

func TestProgramMemberService_GetInvite_EmailInvite(t *testing.T) {
	svc, rec := emailInviteSvc(emailInviteRow(models.ProgramMemberStatusInvited), newMentorAccount(), namedProgRepo(models.ProgramStatusPublished), &stubNotifier{})
	got, err := svc.GetInvite(context.Background(), emailInviteToken(t), newMentorCaller)
	if err != nil {
		t.Fatalf("GetInvite: %v", err)
	}
	if *got != wantInvite {
		t.Errorf("got %+v; want %+v", *got, wantInvite)
	}
	if rec.claimedBy != "" || rec.updatedTo != nil {
		t.Errorf("claimed by %q, updated to %v; want no write", rec.claimedBy, rec.updatedTo)
	}
}

func TestProgramMemberService_GetInvite_EmailInviteErrors(t *testing.T) {
	claimed := emailInviteRow(models.ProgramMemberStatusApproved)
	claimed.UserID = "user-9"

	for name, tc := range map[string]struct {
		row    *models.ProgramMember
		dir    domain.AccountDirectory
		caller models.UserPrincipal
		want   error
	}{
		"address belongs to another account": {row: emailInviteRow(models.ProgramMemberStatusInvited), dir: &stubDirectory{usernames: map[string]string{emailInviteAddress: "someone-else"}}, caller: newMentorCaller, want: domain.ErrForbidden},
		"caller has no LFID":                 {row: emailInviteRow(models.ProgramMemberStatusInvited), dir: newMentorAccount(), caller: models.UserPrincipal{UserID: "user-9"}, want: domain.ErrForbidden},
		"invitation removed":                 {dir: newMentorAccount(), caller: newMentorCaller, want: domain.ErrInvalidInput},
		"invitation already accepted":        {row: claimed, dir: newMentorAccount(), caller: newMentorCaller, want: domain.ErrInvalidInput},
		"auth-service down":                  {row: emailInviteRow(models.ProgramMemberStatusInvited), dir: &stubDirectory{err: domain.ErrUpstreamUnavailable}, caller: newMentorCaller, want: domain.ErrUpstreamUnavailable},
	} {
		svc, _ := emailInviteSvc(tc.row, tc.dir, namedProgRepo(models.ProgramStatusPublished), &stubNotifier{})
		if got, err := svc.GetInvite(context.Background(), emailInviteToken(t), tc.caller); !errors.Is(err, tc.want) || got != nil {
			t.Errorf("%s: got %+v, err = %v; want %v", name, got, err, tc.want)
		}
	}
}

func TestProgramMemberService_GetInvite_ProgramLookupFails(t *testing.T) {
	progRepo := &stubProgRepo{getByID: func(context.Context, string) (*models.Program, error) {
		return nil, domain.ErrProgramNotFound
	}}
	svc := newMemberSvc(userInviteRepo(memberStatus(models.ProgramMemberStatusInvited)), progRepo, &stubNotifier{})
	if _, err := svc.GetInvite(context.Background(), userInviteToken(t), models.UserPrincipal{UserID: "mentor-1"}); !errors.Is(err, domain.ErrProgramNotFound) {
		t.Errorf("err = %v; want ErrProgramNotFound", err)
	}
}
