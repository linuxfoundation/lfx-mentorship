// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
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
	calls     atomic.Int32
}

func (d *stubDirectory) UsernameByEmail(_ context.Context, email string) (string, error) {
	d.calls.Add(1)
	if d.err != nil {
		return "", d.err
	}
	if u, ok := d.usernames[email]; ok {
		return u, nil
	}
	return "", domain.ErrAccountNotFound
}

func (d *stubDirectory) Account(_ context.Context, username string) (*models.LFAccount, error) {
	d.calls.Add(1)
	if d.err != nil {
		return nil, d.err
	}
	if a, ok := d.accounts[username]; ok {
		return a, nil
	}
	return nil, domain.ErrAccountNotFound
}

func (d *stubDirectory) PrimaryEmail(_ context.Context, username string) (string, error) {
	d.calls.Add(1)
	if d.err != nil {
		return "", d.err
	}
	if e, ok := d.emails[username]; ok {
		return e, nil
	}
	return "", domain.ErrAccountNotFound
}

type stubInviteeUsers struct {
	stubLFIDUsers
	upserted  *models.UserCreateInput
	upsertErr error
	found     []*models.User
}

func (s *stubInviteeUsers) UpsertByLFID(_ context.Context, in models.UserCreateInput) (*models.User, error) {
	if s.upsertErr != nil {
		return nil, s.upsertErr
	}
	s.upserted = &in
	return &models.User{ID: "new-user", LFID: in.LFID, Email: in.Email}, nil
}

func (s *stubInviteeUsers) SearchCandidates(context.Context, string, int) ([]*models.User, error) {
	return s.found, nil
}

func newDirectory() *stubDirectory {
	ada := &models.LFAccount{Username: "ada", Name: ptr("Ada Lovelace"), AvatarURL: ptr("https://example.org/ada.png")}
	return &stubDirectory{
		usernames: map[string]string{"ada@example.org": "ada", "ada.primary@example.org": "ada", "mentor@example.org": "mentor-lfid"},
		// "Ada" stands in for Auth0 matching a username in another casing.
		accounts: map[string]*models.LFAccount{"ada": ada, "Ada": ada},
		emails:   map[string]string{"ada": "ada.primary@example.org", "Ada": "ada.primary@example.org", "mentor-lfid": "mentor@example.org"},
	}
}

func newInviteSvc(users *stubInviteeUsers, dir domain.AccountDirectory, memberRepo *stubMemberRepo, n *stubNotifier) *service.ProgramMemberService {
	users.ids = map[string]string{"mentor-lfid": "mentor-1", "legacy+user": "legacy-1"}
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

func TestProgramMemberService_Create_KeysNewUserByStoredUsername(t *testing.T) {
	var captured models.ProgramMemberCreateInput
	users := &stubInviteeUsers{}
	_, err := newInviteSvc(users, newDirectory(), capturingMemberRepo(&captured), &stubNotifier{}).Create(context.Background(), "prog-1",
		models.ProgramMemberCreateInput{LFID: "Ada", MemberType: models.MemberTypeMentor})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if users.upserted == nil || *users.upserted.LFID != "ada" {
		t.Errorf("upserted = %+v; want the row keyed by ada, as sign-in will", users.upserted)
	}
}

func TestProgramMemberService_Create_MixedCaseLFIDFindsExistingUser(t *testing.T) {
	var captured models.ProgramMemberCreateInput
	users := &stubInviteeUsers{}
	svc := newInviteSvc(users, newDirectory(), capturingMemberRepo(&captured), &stubNotifier{})
	users.ids["ada"] = "ada-1"
	_, err := svc.Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{LFID: "Ada", MemberType: models.MemberTypeMentor})
	if err != nil || captured.UserID != "ada-1" || users.upserted != nil {
		t.Errorf("err = %v, user_id = %q, upserted = %v; want existing ada-1 and no upsert", err, captured.UserID, users.upserted)
	}
}

func TestProgramMemberService_Create_ExistingUserSkipsAuthService(t *testing.T) {
	for name, lfid := range map[string]string{
		"known lfid":  "mentor-lfid",
		"legacy lfid": "legacy+user",
	} {
		var captured models.ProgramMemberCreateInput
		users := &stubInviteeUsers{}
		dir := &stubDirectory{err: domain.ErrUpstreamUnavailable}
		_, err := newInviteSvc(users, dir, capturingMemberRepo(&captured), &stubNotifier{}).Create(context.Background(), "prog-1",
			models.ProgramMemberCreateInput{LFID: lfid, MemberType: models.MemberTypeMentor})
		if err != nil || captured.UserID == "" || users.upserted != nil || dir.calls.Load() != 0 {
			t.Errorf("%s: err = %v, user_id = %q, upserted = %v, calls = %d; want the local user, no write and no auth-service call",
				name, err, captured.UserID, users.upserted, dir.calls.Load())
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

func TestProgramMemberService_Create_PrimaryEmailHeldByAnotherUser(t *testing.T) {
	created := false
	memberRepo := &stubMemberRepo{create: func(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
		created = true
		return &models.ProgramMember{}, nil
	}}
	users := &stubInviteeUsers{upsertErr: domain.ErrEmailInUse}
	_, err := newInviteSvc(users, newDirectory(), memberRepo, &stubNotifier{}).Create(context.Background(), "prog-1",
		models.ProgramMemberCreateInput{LFID: "ada", MemberType: models.MemberTypeMentor})
	if !errors.Is(err, domain.ErrIneligible) || created {
		t.Errorf("err = %v, created = %v; want ErrIneligible and no member", err, created)
	}
}

func TestProgramMemberService_Create_InviteeErrors(t *testing.T) {
	badStatus := models.ProgramMemberStatus("bogus")
	for name, tc := range map[string]struct {
		input     models.ProgramMemberCreateInput
		directory *stubDirectory
		noDir     bool
		want      error
	}{
		"unknown email":            {input: models.ProgramMemberCreateInput{Email: ptr("nobody@example.org")}, directory: newDirectory(), want: domain.ErrIneligible},
		"unknown lfid":             {input: models.ProgramMemberCreateInput{LFID: "nobody"}, directory: newDirectory(), want: domain.ErrIneligible},
		"invalid email":            {input: models.ProgramMemberCreateInput{Email: ptr("not an email")}, directory: newDirectory(), want: domain.ErrInvalidInput},
		"uuid lfid":                {input: models.ProgramMemberCreateInput{LFID: "4f9c1f4e-7d0a-4c43-9d0e-1b2f3c4d5e6f"}, directory: newDirectory(), want: domain.ErrInvalidInput},
		"directory down":           {input: models.ProgramMemberCreateInput{LFID: "ada"}, directory: &stubDirectory{err: domain.ErrUpstreamUnavailable}, want: domain.ErrUpstreamUnavailable},
		"email without dir":        {input: models.ProgramMemberCreateInput{Email: ptr("ada@example.org")}, noDir: true, want: domain.ErrUpstreamUnavailable},
		"lfid without dir":         {input: models.ProgramMemberCreateInput{LFID: "ada"}, noDir: true, want: domain.ErrIneligible},
		"nothing identifies":       {input: models.ProgramMemberCreateInput{Email: ptr("  ")}, directory: newDirectory(), want: domain.ErrInvalidInput},
		"invalid status, new lfid": {input: models.ProgramMemberCreateInput{LFID: "ada", Status: &badStatus}, directory: newDirectory(), want: domain.ErrInvalidInput},
		"invalid status, email":    {input: models.ProgramMemberCreateInput{Email: ptr("mentor@example.org"), Status: &badStatus}, directory: newDirectory(), want: domain.ErrInvalidInput},
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
		if (name == "uuid lfid" || strings.HasPrefix(name, "invalid status")) && tc.directory.calls.Load() != 0 {
			t.Errorf("%s reached auth-service %d times", name, tc.directory.calls.Load())
		}
	}
}

func TestProgramMemberService_SearchCandidates(t *testing.T) {
	local := &models.User{LFID: ptr("ada-local"), Name: ptr("Ada Local"), Email: ptr("ada.local@example.org")}
	search := func(users *stubInviteeUsers, dir domain.AccountDirectory, query string) ([]*models.MentorCandidate, error) {
		return newInviteSvc(users, dir, &stubMemberRepo{}, &stubNotifier{}).SearchCandidates(context.Background(), "prog-1", query)
	}

	t.Run("short query", func(t *testing.T) {
		_, err := search(&stubInviteeUsers{}, newDirectory(), " a ")
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("err = %v; want ErrInvalidInput", err)
		}
	})

	t.Run("unpublished program", func(t *testing.T) {
		progRepo := &stubProgRepo{getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusPending}, nil
		}}
		dir := newDirectory()
		_, err := service.NewProgramMemberService(&stubMemberRepo{}, progRepo, &stubInviteeUsers{}, dir, &stubNotifier{}, "").
			SearchCandidates(context.Background(), "prog-1", "ada@example.org")
		if !errors.Is(err, domain.ErrInvalidInput) || dir.calls.Load() != 0 {
			t.Fatalf("err = %v, calls = %d; want ErrInvalidInput and no auth-service call", err, dir.calls.Load())
		}
	})

	t.Run("email resolves only to the account auth-service names", func(t *testing.T) {
		claimed := &models.User{LFID: ptr("impostor"), Name: ptr("Impostor"), Email: ptr("ada@example.org")}
		got, err := search(&stubInviteeUsers{found: []*models.User{claimed, local}}, newDirectory(), "ada@example.org")
		if err != nil || len(got) != 1 || got[0].LFID != "ada" || *got[0].Name != "Ada Lovelace" {
			t.Fatalf("got %v, err %v; want only ada", got, err)
		}
	})

	t.Run("email errors", func(t *testing.T) {
		got, err := search(&stubInviteeUsers{found: []*models.User{local}}, newDirectory(), "ada.local@example.org")
		if err != nil || len(got) != 0 {
			t.Errorf("unknown email: got %v, err %v; want empty", got, err)
		}
		_, err = search(&stubInviteeUsers{found: []*models.User{local}}, nil, "ada.local@example.org")
		if !errors.Is(err, domain.ErrUpstreamUnavailable) {
			t.Errorf("no directory: err = %v; want ErrUpstreamUnavailable", err)
		}
		_, err = search(&stubInviteeUsers{}, &stubDirectory{err: domain.ErrUpstreamUnavailable}, "ada@example.org")
		if !errors.Is(err, domain.ErrUpstreamUnavailable) {
			t.Errorf("directory down: err = %v; want ErrUpstreamUnavailable", err)
		}
	})

	t.Run("name query never reaches auth-service", func(t *testing.T) {
		dir := newDirectory()
		got, err := search(&stubInviteeUsers{found: []*models.User{local}}, dir, "Ada Lo")
		if err != nil || len(got) != 1 || dir.calls.Load() != 0 {
			t.Fatalf("got %v, err %v, calls %d; want local only", got, err, dir.calls.Load())
		}
	})

	t.Run("unknown account returns local results", func(t *testing.T) {
		got, err := search(&stubInviteeUsers{}, newDirectory(), "nobody")
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v; want empty", got, err)
		}
	})

	t.Run("LF account is kept within the limit", func(t *testing.T) {
		full := make([]*models.User, 10)
		for i := range full {
			full[i] = &models.User{LFID: ptr(fmt.Sprintf("ada-%d", i))}
		}
		got, err := search(&stubInviteeUsers{found: full}, newDirectory(), "ada")
		if err != nil || len(got) != 10 || got[0].LFID != "ada" || got[9].LFID != "ada-8" {
			t.Fatalf("got %d candidates, err %v; want ada then the first nine local matches", len(got), err)
		}
	})

	t.Run("auth-service failure keeps the local results", func(t *testing.T) {
		got, err := search(&stubInviteeUsers{found: []*models.User{local}}, &stubDirectory{err: domain.ErrUpstreamUnavailable}, "grace")
		if err != nil || len(got) != 1 || got[0].LFID != "ada-local" {
			t.Fatalf("got %v, err %v; want the local match", got, err)
		}
	})
}
