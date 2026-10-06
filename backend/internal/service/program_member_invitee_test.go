// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

type stubDirectory struct {
	usernames map[string]string
	accounts  map[string]*models.LFAccount
	emails    map[string]string
	err       error
	calls     int
}

func (d *stubDirectory) UsernameByEmail(_ context.Context, email string) (string, error) {
	d.calls++
	if d.err != nil {
		return "", d.err
	}
	if u, ok := d.usernames[email]; ok {
		return u, nil
	}
	return "", domain.ErrAccountNotFound
}

func (d *stubDirectory) Account(_ context.Context, username string) (*models.LFAccount, error) {
	d.calls++
	if d.err != nil {
		return nil, d.err
	}
	if a, ok := d.accounts[username]; ok {
		return a, nil
	}
	return nil, domain.ErrAccountNotFound
}

func (d *stubDirectory) PrimaryEmail(_ context.Context, username string) (string, error) {
	d.calls++
	if e, ok := d.emails[username]; ok {
		return e, nil
	}
	return "", domain.ErrAccountNotFound
}

type stubInviteeUsers struct {
	stubLFIDUsers
	upserted *models.UserCreateInput
	found    []*models.User
}

func (s *stubInviteeUsers) UpsertByLFID(_ context.Context, in models.UserCreateInput) (*models.User, error) {
	s.upserted = &in
	return &models.User{ID: "new-user", LFID: in.LFID}, nil
}

func (s *stubInviteeUsers) SearchCandidates(context.Context, string, int) ([]*models.User, error) {
	return s.found, nil
}

func newDirectory() *stubDirectory {
	return &stubDirectory{
		usernames: map[string]string{"ada@example.org": "ada", "mentor@example.org": "mentor-lfid"},
		accounts:  map[string]*models.LFAccount{"ada": {Username: "ada", Name: ptr("Ada Lovelace"), AvatarURL: ptr("https://example.org/ada.png")}},
		emails:    map[string]string{"ada": "ada.primary@example.org"},
	}
}

func newInviteSvc(users *stubInviteeUsers, dir domain.AccountDirectory, memberRepo *stubMemberRepo, n *stubNotifier) *service.ProgramMemberService {
	users.ids = map[string]string{"mentor-lfid": "mentor-1"}
	return service.NewProgramMemberService(memberRepo, publishedProgRepo(), users, dir, n, "test-secret")
}

func capturingMemberRepo(captured *models.ProgramMemberCreateInput) *stubMemberRepo {
	return &stubMemberRepo{
		create: func(_ context.Context, _ string, in models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
			*captured = in
			return &models.ProgramMember{UserID: in.UserID, Status: in.Status}, nil
		},
	}
}

func TestProgramMemberService_Create_CreatesUserForNewLFAccount(t *testing.T) {
	for name, input := range map[string]models.ProgramMemberCreateInput{
		"by lfid":  {LFID: "ada", MemberType: models.MemberTypeMentor},
		"by email": {Email: ptr(" ada@example.org "), MemberType: models.MemberTypeMentor},
	} {
		var captured models.ProgramMemberCreateInput
		users := &stubInviteeUsers{}
		n := &stubNotifier{}
		_, err := newInviteSvc(users, newDirectory(), capturingMemberRepo(&captured), n).Create(context.Background(), "prog-1", input)
		if err != nil {
			t.Fatalf("%s: Create: %v", name, err)
		}
		if users.upserted == nil || *users.upserted.LFID != "ada" || *users.upserted.Email != "ada.primary@example.org" || *users.upserted.Name != "Ada Lovelace" {
			t.Errorf("%s: upserted = %+v; want ada with her primary email and name", name, users.upserted)
		}
		if captured.UserID != "new-user" {
			t.Errorf("%s: user_id = %q; want new-user", name, captured.UserID)
		}
		if n.mentorInvitedCalls != 1 {
			t.Errorf("%s: NotifyMentorInvited called %d times; want 1", name, n.mentorInvitedCalls)
		}
	}
}

func TestProgramMemberService_Create_EmailOfExistingUserSkipsUpsert(t *testing.T) {
	var captured models.ProgramMemberCreateInput
	users := &stubInviteeUsers{}
	_, err := newInviteSvc(users, newDirectory(), capturingMemberRepo(&captured), &stubNotifier{}).Create(context.Background(), "prog-1",
		models.ProgramMemberCreateInput{Email: ptr("mentor@example.org"), MemberType: models.MemberTypeMentor})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if captured.UserID != "mentor-1" || users.upserted != nil {
		t.Errorf("user_id = %q, upserted = %v; want the existing mentor-1 and no upsert", captured.UserID, users.upserted)
	}
}

func TestProgramMemberService_Create_InviteeErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		input     models.ProgramMemberCreateInput
		directory *stubDirectory
		noDir     bool
		want      error
	}{
		"unknown email":      {input: models.ProgramMemberCreateInput{Email: ptr("nobody@example.org")}, directory: newDirectory(), want: domain.ErrIneligible},
		"unknown lfid":       {input: models.ProgramMemberCreateInput{LFID: "nobody"}, directory: newDirectory(), want: domain.ErrIneligible},
		"invalid email":      {input: models.ProgramMemberCreateInput{Email: ptr("not an email")}, directory: newDirectory(), want: domain.ErrInvalidInput},
		"uuid lfid":          {input: models.ProgramMemberCreateInput{LFID: "4f9c1f4e-7d0a-4c43-9d0e-1b2f3c4d5e6f"}, directory: newDirectory(), want: domain.ErrInvalidInput},
		"directory down":     {input: models.ProgramMemberCreateInput{LFID: "ada"}, directory: &stubDirectory{err: domain.ErrUpstreamUnavailable}, want: domain.ErrUpstreamUnavailable},
		"email without dir":  {input: models.ProgramMemberCreateInput{Email: ptr("ada@example.org")}, noDir: true, want: domain.ErrUpstreamUnavailable},
		"lfid without dir":   {input: models.ProgramMemberCreateInput{LFID: "ada"}, noDir: true, want: domain.ErrIneligible},
		"nothing identifies": {input: models.ProgramMemberCreateInput{Email: ptr("  ")}, directory: newDirectory(), want: domain.ErrInvalidInput},
	} {
		tc.input.MemberType = models.MemberTypeMentor
		created := false
		memberRepo := &stubMemberRepo{
			create: func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
				created = true
				return &models.ProgramMember{}, nil
			},
		}
		var dir domain.AccountDirectory
		if !tc.noDir {
			dir = tc.directory
		}
		users := &stubInviteeUsers{}
		_, err := newInviteSvc(users, dir, memberRepo, &stubNotifier{}).Create(context.Background(), "prog-1", tc.input)
		if !errors.Is(err, tc.want) || created || users.upserted != nil {
			t.Errorf("%s: err = %v, created = %v, upserted = %v; want %v and no writes", name, err, created, users.upserted != nil, tc.want)
		}
		if name == "uuid lfid" && tc.directory.calls != 0 {
			t.Errorf("uuid lfid reached auth-service %d times", tc.directory.calls)
		}
	}
}

func TestProgramMemberService_SearchCandidates(t *testing.T) {
	local := &models.User{LFID: ptr("ada-local"), Name: ptr("Ada Local"), Email: ptr("ada.local@example.org")}

	t.Run("short query", func(t *testing.T) {
		_, err := newInviteSvc(&stubInviteeUsers{}, newDirectory(), &stubMemberRepo{}, &stubNotifier{}).SearchCandidates(context.Background(), " a ")
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("err = %v; want ErrInvalidInput", err)
		}
	})

	t.Run("exact local match skips auth-service", func(t *testing.T) {
		dir := newDirectory()
		got, err := newInviteSvc(&stubInviteeUsers{found: []*models.User{local}}, dir, &stubMemberRepo{}, &stubNotifier{}).SearchCandidates(context.Background(), "ADA.local@example.org")
		if err != nil || len(got) != 1 || got[0].LFID != "ada-local" || dir.calls != 0 {
			t.Fatalf("got %v, err %v, calls %d; want only ada-local and no auth-service call", got, err, dir.calls)
		}
	})

	t.Run("email finds an LF account first", func(t *testing.T) {
		got, err := newInviteSvc(&stubInviteeUsers{found: []*models.User{local}}, newDirectory(), &stubMemberRepo{}, &stubNotifier{}).SearchCandidates(context.Background(), "ada@example.org")
		if err != nil || len(got) != 2 || got[0].LFID != "ada" || *got[0].Name != "Ada Lovelace" || got[1].LFID != "ada-local" {
			t.Fatalf("got %v, err %v; want ada then ada-local", got, err)
		}
	})

	t.Run("name query never reaches auth-service", func(t *testing.T) {
		dir := newDirectory()
		got, err := newInviteSvc(&stubInviteeUsers{found: []*models.User{local}}, dir, &stubMemberRepo{}, &stubNotifier{}).SearchCandidates(context.Background(), "Ada Lo")
		if err != nil || len(got) != 1 || dir.calls != 0 {
			t.Fatalf("got %v, err %v, calls %d; want local only", got, err, dir.calls)
		}
	})

	t.Run("unknown account returns local results", func(t *testing.T) {
		got, err := newInviteSvc(&stubInviteeUsers{}, newDirectory(), &stubMemberRepo{}, &stubNotifier{}).SearchCandidates(context.Background(), "nobody")
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v; want empty", got, err)
		}
	})

	t.Run("auth-service failure is an error", func(t *testing.T) {
		_, err := newInviteSvc(&stubInviteeUsers{}, &stubDirectory{err: domain.ErrUpstreamUnavailable}, &stubMemberRepo{}, &stubNotifier{}).SearchCandidates(context.Background(), "ada")
		if !errors.Is(err, domain.ErrUpstreamUnavailable) {
			t.Fatalf("err = %v; want ErrUpstreamUnavailable", err)
		}
	})
}
