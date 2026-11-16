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
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

const emailInviteAddress = "new.mentor@example.org"

// emailInviteRow is an open email invitation: a mentor row with no user.
func emailInviteRow(status models.ProgramMemberStatus) *models.ProgramMember {
	return &models.ProgramMember{
		ID:         "invite-1",
		ProgramID:  "prog-1",
		MemberType: models.MemberTypeMentor,
		Status:     &status,
		Email:      ptr(emailInviteAddress),
	}
}

func emailInviteToken(t *testing.T) string {
	t.Helper()
	token, err := auth.GenerateEmailInviteToken("prog-1", "invite-1", "test-secret")
	if err != nil {
		t.Fatalf("GenerateEmailInviteToken: %v", err)
	}
	return token
}

// newMentorAccount is the LF account the invitee signs up with: the invited address resolves to it.
func newMentorAccount() *stubDirectory {
	return &stubDirectory{usernames: map[string]string{emailInviteAddress: "new-mentor"}}
}

var newMentorCaller = models.UserPrincipal{UserID: "user-9", LFID: "new-mentor"}

func TestProgramMemberService_Create_EmailWithNoAccountBecomesEmailInvite(t *testing.T) {
	var captured models.ProgramMemberCreateInput
	n := &stubNotifier{}
	users := &stubInviteeUsers{}
	m, err := newInviteSvc(users, newDirectory(), capturingMemberRepo(&captured), n).Create(context.Background(), "prog-1",
		models.ProgramMemberCreateInput{Email: ptr(" " + emailInviteAddress + " "), MemberType: models.MemberTypeMentor, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if captured.UserID != "" || captured.Email == nil || *captured.Email != emailInviteAddress || captured.Status == nil || *captured.Status != models.ProgramMemberStatusInvited {
		t.Errorf("created %+v; want an invited row with no user and the trimmed address", captured)
	}
	if m == nil || users.upserted != nil {
		t.Errorf("member = %v, upserted = %v; want a member and no user", m, users.upserted)
	}
	if len(n.emailInvites) != 1 || n.emailInvites[0] != emailInviteAddress || n.mentorInvitedCalls != 0 {
		t.Errorf("email invites = %v, account invites = %d; want one email invite to the address", n.emailInvites, n.mentorInvitedCalls)
	}
}

func TestProgramMemberService_Create_EmailWithNoAccountStaysIneligibleForAdmins(t *testing.T) {
	created := false
	memberRepo := &stubMemberRepo{create: func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
		created = true
		return &models.ProgramMember{}, nil
	}}
	_, err := newInviteSvc(&stubInviteeUsers{}, newDirectory(), memberRepo, &stubNotifier{}).Create(context.Background(), "prog-1",
		models.ProgramMemberCreateInput{Email: ptr(emailInviteAddress), MemberType: models.MemberTypeProgramAdmin})
	if !errors.Is(err, domain.ErrIneligible) || created {
		t.Errorf("err = %v, created = %v; want ErrIneligible and no member", err, created)
	}
}

func TestProgramMemberService_Create_EmailInviteAgain(t *testing.T) {
	for name, tc := range map[string]struct {
		status     models.ProgramMemberStatus
		wantUpdate bool
	}{
		"open invitation resends":     {status: models.ProgramMemberStatusInvited},
		"declined invitation reopens": {status: models.ProgramMemberStatusDeclined, wantUpdate: true},
	} {
		var updatedFrom []models.ProgramMemberStatus
		created := false
		n := &stubNotifier{}
		memberRepo := &stubMemberRepo{
			findEmailInvite: func(_ context.Context, _, address string) (*models.ProgramMember, error) {
				if address != emailInviteAddress {
					t.Errorf("%s: looked up %q", name, address)
				}
				return emailInviteRow(tc.status), nil
			},
			updateIfStatus: func(_ context.Context, id string, from []models.ProgramMemberStatus, in models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
				updatedFrom = from
				row := emailInviteRow(*in.Status)
				return row, nil
			},
			create: func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
				created = true
				return &models.ProgramMember{}, nil
			},
		}
		m, err := newInviteSvc(&stubInviteeUsers{}, newDirectory(), memberRepo, n).Create(context.Background(), "prog-1",
			models.ProgramMemberCreateInput{Email: ptr(emailInviteAddress), MemberType: models.MemberTypeMentor})
		if err != nil || created {
			t.Fatalf("%s: err = %v, created = %v; want the existing row reused", name, err, created)
		}
		if (updatedFrom != nil) != tc.wantUpdate {
			t.Errorf("%s: updated from %v; want update = %v", name, updatedFrom, tc.wantUpdate)
		}
		if m.Status == nil || *m.Status != models.ProgramMemberStatusInvited || len(n.emailInvites) != 1 {
			t.Errorf("%s: status = %v, email invites = %v; want invited and one resend", name, m.Status, n.emailInvites)
		}
	}
}

func TestProgramMemberService_Create_AccountInviteDropsStaleEmailInvite(t *testing.T) {
	var deleted string
	var captured models.ProgramMemberCreateInput
	memberRepo := capturingMemberRepo(&captured)
	memberRepo.findEmailInvite = func(context.Context, string, string) (*models.ProgramMember, error) {
		return emailInviteRow(models.ProgramMemberStatusInvited), nil
	}
	memberRepo.delete = func(context.Context, string) error {
		t.Fatal("Delete called; a stale email invitation must go through DeleteEmailInvite, which spares a claimed row")
		return nil
	}
	memberRepo.deleteEmailInvite = func(_ context.Context, id string) error {
		deleted = id
		return nil
	}
	_, err := newInviteSvc(&stubInviteeUsers{}, newDirectory(), memberRepo, &stubNotifier{}).Create(context.Background(), "prog-1",
		models.ProgramMemberCreateInput{Email: ptr("mentor@example.org"), MemberType: models.MemberTypeMentor})
	if err != nil || captured.UserID != "mentor-1" {
		t.Fatalf("err = %v, user_id = %q; want the account invited", err, captured.UserID)
	}
	if deleted != "invite-1" {
		t.Errorf("deleted %q; want the superseded email invitation", deleted)
	}
}

// The invitee can claim the email invitation between the lookup and the delete;
// the delete then finds no unclaimed row, and the account invite still succeeds.
func TestProgramMemberService_Create_AccountInviteIgnoresEmailInviteClaimedMeanwhile(t *testing.T) {
	var captured models.ProgramMemberCreateInput
	memberRepo := capturingMemberRepo(&captured)
	memberRepo.findEmailInvite = func(context.Context, string, string) (*models.ProgramMember, error) {
		return emailInviteRow(models.ProgramMemberStatusInvited), nil
	}
	memberRepo.deleteEmailInvite = func(context.Context, string) error { return domain.ErrProgramMemberNotFound }
	_, err := newInviteSvc(&stubInviteeUsers{}, newDirectory(), memberRepo, &stubNotifier{}).Create(context.Background(), "prog-1",
		models.ProgramMemberCreateInput{Email: ptr("mentor@example.org"), MemberType: models.MemberTypeMentor})
	if err != nil || captured.UserID != "mentor-1" {
		t.Fatalf("err = %v, user_id = %q; want the account invited", err, captured.UserID)
	}
}

func TestProgramMemberService_Update_EmailInviteCanOnlyBeRevoked(t *testing.T) {
	for _, to := range []models.ProgramMemberStatus{models.ProgramMemberStatusApproved, models.ProgramMemberStatusPending, models.ProgramMemberStatusDeclined} {
		memberRepo := approvedProgramAdmin()
		memberRepo.getByID = func(context.Context, string) (*models.ProgramMember, error) {
			return emailInviteRow(models.ProgramMemberStatusInvited), nil
		}
		svc := service.NewProgramMemberService(memberRepo, publishedProgRepo(), &stubInviteeUsers{}, nil, &stubNotifier{}, "test-secret")
		_, err := svc.Update(context.Background(), "prog-1", "invite-1", models.ProgramMemberUpdateInput{Status: &to}, "admin-1")
		if to == models.ProgramMemberStatusDeclined {
			if err != nil {
				t.Errorf("revoke: err = %v; want nil", err)
			}
			continue
		}
		if !errors.Is(err, domain.ErrInvalidStateTransition) {
			t.Errorf("%s: err = %v; want ErrInvalidStateTransition", to, err)
		}
	}
}

func TestProgramMemberService_Update_EmailInviteAddressCannotChange(t *testing.T) {
	declined := models.ProgramMemberStatusDeclined
	for name, in := range map[string]models.ProgramMemberUpdateInput{
		"email only":     {Email: ptr("other@example.org")},
		"email + revoke": {Email: ptr("other@example.org"), Status: &declined},
	} {
		memberRepo := approvedProgramAdmin()
		memberRepo.getByID = func(context.Context, string) (*models.ProgramMember, error) {
			return emailInviteRow(models.ProgramMemberStatusInvited), nil
		}
		memberRepo.update = func(context.Context, string, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
			t.Fatalf("%s: Update called; the address must not change", name)
			return nil, nil
		}
		memberRepo.updateIfStatus = func(context.Context, string, []models.ProgramMemberStatus, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
			t.Fatalf("%s: UpdateIfStatus called; the address must not change", name)
			return nil, nil
		}
		svc := service.NewProgramMemberService(memberRepo, publishedProgRepo(), &stubInviteeUsers{}, nil, &stubNotifier{}, "test-secret")
		_, err := svc.Update(context.Background(), "prog-1", "invite-1", in, "admin-1")
		if !errors.Is(err, domain.ErrInvalidStateTransition) {
			t.Errorf("%s: err = %v; want ErrInvalidStateTransition", name, err)
		}
	}
}

// emailInviteRecorder records the claim and status writes emailInviteSvc's repository receives.
type emailInviteRecorder struct {
	claimedBy string
	updatedTo *models.ProgramMemberStatus
}

// emailInviteSvc is a service whose member repository holds row as invite-1.
func emailInviteSvc(row *models.ProgramMember, dir domain.AccountDirectory, progRepo *stubProgRepo, n *stubNotifier) (*service.ProgramMemberService, *emailInviteRecorder) {
	rec := &emailInviteRecorder{}
	memberRepo := &stubMemberRepo{
		getByID: func(_ context.Context, id string) (*models.ProgramMember, error) {
			if row == nil || id != row.ID {
				return nil, domain.ErrProgramMemberNotFound
			}
			return row, nil
		},
		claimEmailInvite: func(_ context.Context, id, userID string) (*models.ProgramMember, error) {
			rec.claimedBy = userID
			approved := models.ProgramMemberStatusApproved
			return &models.ProgramMember{ID: id, ProgramID: "prog-1", UserID: userID, Status: &approved}, nil
		},
		updateIfStatus: func(_ context.Context, id string, _ []models.ProgramMemberStatus, in models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
			rec.updatedTo = in.Status
			return &models.ProgramMember{ID: id, Status: in.Status}, nil
		},
	}
	return service.NewProgramMemberService(memberRepo, progRepo, &stubInviteeUsers{}, dir, n, "test-secret"), rec
}

func TestProgramMemberService_AcceptInvite_EmailInvite(t *testing.T) {
	n := &stubNotifier{}
	svc, rec := emailInviteSvc(emailInviteRow(models.ProgramMemberStatusInvited), newMentorAccount(), publishedProgRepo(), n)
	m, err := svc.AcceptInvite(context.Background(), emailInviteToken(t), newMentorCaller)
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if rec.claimedBy != "user-9" || m.UserID != "user-9" {
		t.Errorf("claimed by %q, member user %q; want the caller", rec.claimedBy, m.UserID)
	}
	if n.adminMentorAcceptedCalls != 1 {
		t.Errorf("admin accepted notifications = %d; want 1", n.adminMentorAcceptedCalls)
	}
}

func TestProgramMemberService_AcceptInvite_EmailInviteMatchesLFIDWithoutCase(t *testing.T) {
	svc, rec := emailInviteSvc(emailInviteRow(models.ProgramMemberStatusInvited), newMentorAccount(), publishedProgRepo(), &stubNotifier{})
	caller := models.UserPrincipal{UserID: "user-9", LFID: "New-Mentor"}
	if _, err := svc.AcceptInvite(context.Background(), emailInviteToken(t), caller); err != nil || rec.claimedBy != "user-9" {
		t.Fatalf("err = %v, claimed by %q; want the caller", err, rec.claimedBy)
	}
}

func TestProgramMemberService_AnswerEmailInvite_Errors(t *testing.T) {
	otherProgram := emailInviteRow(models.ProgramMemberStatusInvited)
	otherProgram.ProgramID = "prog-2"
	claimed := emailInviteRow(models.ProgramMemberStatusApproved)
	claimed.UserID = "user-9"
	unpublished := &stubProgRepo{getByID: func(_ context.Context, id string) (*models.Program, error) {
		return &models.Program{ID: id, Status: models.ProgramStatusPending}, nil
	}}

	for name, tc := range map[string]struct {
		row        *models.ProgramMember
		dir        domain.AccountDirectory
		progRepo   *stubProgRepo
		caller     models.UserPrincipal
		want       error
		acceptOnly bool
	}{
		"address belongs to another account": {row: emailInviteRow(models.ProgramMemberStatusInvited), dir: &stubDirectory{usernames: map[string]string{emailInviteAddress: "someone-else"}}, caller: newMentorCaller, want: domain.ErrForbidden},
		"address has no verified account":    {row: emailInviteRow(models.ProgramMemberStatusInvited), dir: &stubDirectory{}, caller: newMentorCaller, want: domain.ErrForbidden},
		"caller has no LFID":                 {row: emailInviteRow(models.ProgramMemberStatusInvited), dir: newMentorAccount(), caller: models.UserPrincipal{UserID: "user-9"}, want: domain.ErrForbidden},
		"invitation removed":                 {row: nil, dir: newMentorAccount(), caller: newMentorCaller, want: domain.ErrInvalidInput},
		"invitation already accepted":        {row: claimed, dir: newMentorAccount(), caller: newMentorCaller, want: domain.ErrInvalidInput},
		"invitation declined":                {row: emailInviteRow(models.ProgramMemberStatusDeclined), dir: newMentorAccount(), caller: newMentorCaller, want: domain.ErrInvalidInput},
		"token for another program":          {row: otherProgram, dir: newMentorAccount(), caller: newMentorCaller, want: domain.ErrInvalidInput},
		"auth-service down":                  {row: emailInviteRow(models.ProgramMemberStatusInvited), dir: &stubDirectory{err: domain.ErrUpstreamUnavailable}, caller: newMentorCaller, want: domain.ErrUpstreamUnavailable},
		"program no longer published":        {row: emailInviteRow(models.ProgramMemberStatusInvited), dir: newMentorAccount(), progRepo: unpublished, caller: newMentorCaller, want: domain.ErrConflict, acceptOnly: true},
	} {
		progRepo := tc.progRepo
		if progRepo == nil {
			progRepo = publishedProgRepo()
		}
		n := &stubNotifier{}
		svc, rec := emailInviteSvc(tc.row, tc.dir, progRepo, n)
		if _, err := svc.AcceptInvite(context.Background(), emailInviteToken(t), tc.caller); !errors.Is(err, tc.want) || rec.claimedBy != "" {
			t.Errorf("%s: accept err = %v, claimed by %q; want %v and no claim", name, err, rec.claimedBy, tc.want)
		}
		if tc.acceptOnly {
			continue
		}
		if err := svc.DeclineInvite(context.Background(), emailInviteToken(t), tc.caller); !errors.Is(err, tc.want) || rec.updatedTo != nil {
			t.Errorf("%s: decline err = %v, updated to %v; want %v and no write", name, err, rec.updatedTo, tc.want)
		}
		if n.adminMentorAcceptedCalls+n.adminMentorDeclinedCalls != 0 {
			t.Errorf("%s: admins notified of a refused answer", name)
		}
	}
}

func TestProgramMemberService_DeclineInvite_EmailInvite(t *testing.T) {
	n := &stubNotifier{}
	svc, rec := emailInviteSvc(emailInviteRow(models.ProgramMemberStatusInvited), newMentorAccount(), publishedProgRepo(), n)
	if err := svc.DeclineInvite(context.Background(), emailInviteToken(t), newMentorCaller); err != nil {
		t.Fatalf("DeclineInvite: %v", err)
	}
	if rec.updatedTo == nil || *rec.updatedTo != models.ProgramMemberStatusDeclined || rec.claimedBy != "" {
		t.Errorf("updated to %v, claimed by %q; want declined and no claim", rec.updatedTo, rec.claimedBy)
	}
	if n.adminMentorDeclinedCalls != 1 {
		t.Errorf("admin declined notifications = %d; want 1", n.adminMentorDeclinedCalls)
	}
}

func TestProgramMemberService_AcceptInvite_UserTokenIgnoresEmailPath(t *testing.T) {
	token, err := auth.GenerateInviteToken("prog-1", "mentor-1", "test-secret")
	if err != nil {
		t.Fatalf("GenerateInviteToken: %v", err)
	}
	svc, rec := emailInviteSvc(emailInviteRow(models.ProgramMemberStatusInvited), newMentorAccount(), publishedProgRepo(), &stubNotifier{})
	if _, err := svc.AcceptInvite(context.Background(), token, newMentorCaller); !errors.Is(err, domain.ErrForbidden) || rec.claimedBy != "" {
		t.Errorf("err = %v, claimed by %q; want a user token for someone else refused", err, rec.claimedBy)
	}
}
