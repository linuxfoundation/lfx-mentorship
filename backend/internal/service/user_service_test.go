// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

type stubUserRepository struct {
	delete    func(context.Context, string) error
	update    func(context.Context, string, models.UserUpdateInput) (*models.User, error)
	bootstrap func(context.Context, models.UserCreateInput, models.UserIdentityFill) (*models.User, error)
	getByLFID func(context.Context, string) (*models.User, error)
}

func (s *stubUserRepository) GetByID(context.Context, string) (*models.User, error) {
	return &models.User{}, nil
}

func (s *stubUserRepository) GetByLFID(ctx context.Context, lfid string) (*models.User, error) {
	if s.getByLFID != nil {
		return s.getByLFID(ctx, lfid)
	}
	return nil, domain.ErrUserNotFound
}

func (s *stubUserRepository) List(context.Context, models.UserFilter) ([]*models.User, *models.PaginationMeta, error) {
	return nil, &models.PaginationMeta{}, nil
}

func (s *stubUserRepository) Create(context.Context, models.UserCreateInput) (*models.User, error) {
	return &models.User{}, nil
}

func (s *stubUserRepository) UpsertByLFID(context.Context, models.UserCreateInput) (*models.User, error) {
	return &models.User{}, nil
}

func (s *stubUserRepository) BootstrapByLFID(ctx context.Context, input models.UserCreateInput, fill models.UserIdentityFill) (*models.User, error) {
	if s.bootstrap != nil {
		return s.bootstrap(ctx, input, fill)
	}
	return &models.User{}, nil
}

func (s *stubUserRepository) SearchCandidates(context.Context, string, int) ([]*models.User, error) {
	return []*models.User{}, nil
}

func (s *stubUserRepository) Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
	if s.update != nil {
		return s.update(ctx, id, input)
	}
	return &models.User{}, nil
}

func (s *stubUserRepository) Delete(ctx context.Context, id string) error {
	return s.delete(ctx, id)
}

func TestUserService_Delete_AllowsOwner(t *testing.T) {
	called := false
	svc := service.NewUserService(&stubUserRepository{
		delete: func(_ context.Context, id string) error {
			called = id == "user-1"
			return nil
		},
	})

	if err := svc.Delete(context.Background(), "user-1", "user-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !called {
		t.Fatal("expected repository delete for owner")
	}
}

func TestUserService_DeleteRejectsOtherActor(t *testing.T) {
	svc := service.NewUserService(&stubUserRepository{
		delete: func(context.Context, string) error {
			t.Fatal("repository delete should not be called")
			return nil
		},
	})

	if err := svc.Delete(context.Background(), "user-1", "user-2"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v; want ErrForbidden", err)
	}
}

func TestUserService_DeleteRejectsMissingActor(t *testing.T) {
	svc := service.NewUserService(&stubUserRepository{
		delete: func(context.Context, string) error {
			t.Fatal("repository delete should not be called")
			return nil
		},
	})

	if err := svc.Delete(context.Background(), "user-1", ""); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v; want ErrForbidden", err)
	}
}

func TestUserService_UpdateSavesAvatarURL(t *testing.T) {
	avatar := "https://images.example.org/avatar.png"
	var saved *string
	svc := service.NewUserService(&stubUserRepository{
		update: func(_ context.Context, _ string, in models.UserUpdateInput) (*models.User, error) {
			saved = in.AvatarURL
			return &models.User{}, nil
		},
	})
	if _, err := svc.Update(context.Background(), "user-1", models.UserUpdateInput{AvatarURL: &avatar}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if saved == nil || *saved != avatar {
		t.Fatalf("saved avatar_url = %v; want %q", saved, avatar)
	}
}

func TestUserService_UpdateRejectsLFIDMutation(t *testing.T) {
	called := false
	svc := service.NewUserService(&stubUserRepository{
		update: func(context.Context, string, models.UserUpdateInput) (*models.User, error) {
			called = true
			return &models.User{}, nil
		},
	})
	lfid := "attempted-change"
	_, err := svc.Update(context.Background(), "user-1", models.UserUpdateInput{LFID: &lfid})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v; want ErrForbidden", err)
	}
	if called {
		t.Fatal("repository update should not be called when LFID mutation is requested")
	}
}

func TestUserService_UpdateRejectsEmailMutation(t *testing.T) {
	called := false
	svc := service.NewUserService(&stubUserRepository{
		update: func(context.Context, string, models.UserUpdateInput) (*models.User, error) {
			called = true
			return &models.User{}, nil
		},
	})
	email := "victim@example.org"
	_, err := svc.Update(context.Background(), "user-1", models.UserUpdateInput{Email: &email})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v; want ErrForbidden", err)
	}
	if called {
		t.Fatal("repository update should not be called when email mutation is requested")
	}
}

// Bootstrap fills from the LF directory only what the token, the body and the
// stored row all lack, as lfx-crowdfunding's sign-in sync does, and never
// fails the sign-in on a directory problem (#360). Token and body values go
// to the write as input (they replace stored values); directory values go as
// a fill, which the repository applies only to columns still NULL.
func TestUserService_Bootstrap_FillsMissingIdentityFromDirectory(t *testing.T) {
	str := func(v string) *string { return &v }
	fullAccount := &models.LFAccount{Username: "alice", Name: str("Alice Liddell"), GivenName: str("Alice"), FamilyName: str("Liddell"), AvatarURL: str("https://img/alice.png")}
	knownDir := func() *stubDirectory {
		return &stubDirectory{accounts: map[string]*models.LFAccount{"alice": fullAccount}, emails: map[string]string{"alice": "dir@example.org"}}
	}
	for name, tc := range map[string]struct {
		dir       *stubDirectory
		stored    *models.User // nil: first sign-in
		input     models.UserUpdateInput
		wantInput models.UserCreateInput
		wantFill  models.UserIdentityFill
		wantCalls int32
	}{
		"token carried everything: no directory call, nothing to fill": {
			dir:       knownDir(),
			input:     models.UserUpdateInput{Email: str("tok@example.org"), Name: str("Tok Name"), GivenName: str("Tok"), FamilyName: str("Name"), AvatarURL: str("https://img/tok.png")},
			wantInput: models.UserCreateInput{LFID: str("alice"), Email: str("tok@example.org"), Name: str("Tok Name"), GivenName: str("Tok"), FamilyName: str("Name"), AvatarURL: str("https://img/tok.png")},
		},
		"first sign-in, empty token: directory fills name and email": {
			dir:       knownDir(),
			wantInput: models.UserCreateInput{LFID: str("alice")},
			wantFill:  models.UserIdentityFill{Email: str("dir@example.org"), Name: str("Alice Liddell"), GivenName: str("Alice"), FamilyName: str("Liddell"), AvatarURL: str("https://img/alice.png")},
			wantCalls: 2,
		},
		"token email, directory name: email stays an input, name is a fill": {
			dir:       knownDir(),
			input:     models.UserUpdateInput{Email: str("tok@example.org")},
			wantInput: models.UserCreateInput{LFID: str("alice"), Email: str("tok@example.org")},
			wantFill:  models.UserIdentityFill{Name: str("Alice Liddell"), GivenName: str("Alice"), FamilyName: str("Liddell"), AvatarURL: str("https://img/alice.png")},
			wantCalls: 1,
		},
		"stored row complete: directory not asked, nothing filled": {
			dir:       knownDir(),
			stored:    &models.User{LFID: str("alice"), Email: str("stored@example.org"), Name: str("Edited Name"), GivenName: str("Edited"), FamilyName: str("Name"), AvatarURL: str("https://img/edited.png")},
			wantInput: models.UserCreateInput{LFID: str("alice")},
		},
		"stored row lacks only the avatar: directory fills just that": {
			dir:       knownDir(),
			stored:    &models.User{LFID: str("alice"), Email: str("stored@example.org"), Name: str("Edited Name"), GivenName: str("Edited"), FamilyName: str("Name")},
			wantInput: models.UserCreateInput{LFID: str("alice")},
			wantFill:  models.UserIdentityFill{AvatarURL: str("https://img/alice.png")},
			wantCalls: 1,
		},
		"directory down: sign-in still succeeds with what the token had": {
			dir:       &stubDirectory{err: errors.New("nats: no responders")},
			input:     models.UserUpdateInput{Email: str("tok@example.org")},
			wantInput: models.UserCreateInput{LFID: str("alice"), Email: str("tok@example.org")},
			wantCalls: 1,
		},
		"no LF account (M2M client): nothing filled, no error": {
			dir:       &stubDirectory{},
			wantInput: models.UserCreateInput{LFID: str("alice")},
			wantCalls: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var gotInput models.UserCreateInput
			var gotFill models.UserIdentityFill
			repo := &stubUserRepository{bootstrap: func(_ context.Context, in models.UserCreateInput, fill models.UserIdentityFill) (*models.User, error) {
				gotInput, gotFill = in, fill
				return &models.User{LFID: in.LFID}, nil
			}}
			if tc.stored != nil {
				repo.getByLFID = func(context.Context, string) (*models.User, error) { return tc.stored, nil }
			}
			svc := service.NewUserService(repo)
			svc.SetAccountDirectory(tc.dir)

			if _, err := svc.Bootstrap(context.Background(), "alice", tc.input); err != nil {
				t.Fatalf("Bootstrap: %v", err)
			}
			if !reflect.DeepEqual(gotInput, tc.wantInput) {
				g, _ := json.Marshal(gotInput)
				w, _ := json.Marshal(tc.wantInput)
				t.Fatalf("input %s; want %s", g, w)
			}
			if !reflect.DeepEqual(gotFill, tc.wantFill) {
				t.Fatalf("fill %s; want %s", describeFill(gotFill), describeFill(tc.wantFill))
			}
			if calls := tc.dir.calls.Load(); calls != tc.wantCalls {
				t.Errorf("directory calls = %d; want %d", calls, tc.wantCalls)
			}
		})
	}
}

func describeFill(f models.UserIdentityFill) string {
	b, _ := json.Marshal(map[string]*string{"email": f.Email, "name": f.Name, "given_name": f.GivenName, "family_name": f.FamilyName, "avatar_url": f.AvatarURL})
	return string(b)
}

// A directory email another row holds is given up, not taken: the directory
// cannot vouch that the address is verified.
func TestUserService_Bootstrap_DirectoryEmailYieldsOnConflict(t *testing.T) {
	var fills []models.UserIdentityFill
	repo := &stubUserRepository{bootstrap: func(_ context.Context, in models.UserCreateInput, fill models.UserIdentityFill) (*models.User, error) {
		fills = append(fills, fill)
		if fill.Email != nil {
			return nil, domain.ErrEmailInUse
		}
		return &models.User{LFID: in.LFID}, nil
	}}
	svc := service.NewUserService(repo)
	svc.SetAccountDirectory(&stubDirectory{emails: map[string]string{"alice": "shared@example.org"}})

	if _, err := svc.Bootstrap(context.Background(), "alice", models.UserUpdateInput{}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if len(fills) != 2 || fills[0].Email == nil || *fills[0].Email != "shared@example.org" || fills[1].Email != nil {
		t.Fatalf("fills = %+v; want one attempt with the directory email, then one without", fills)
	}
}

// A conflict on a token email is not retried: it was released first, so it is a real error.
func TestUserService_Bootstrap_TokenEmailConflictIsAnError(t *testing.T) {
	email := "tok@example.org"
	calls := 0
	repo := &stubUserRepository{bootstrap: func(context.Context, models.UserCreateInput, models.UserIdentityFill) (*models.User, error) {
		calls++
		return nil, domain.ErrEmailInUse
	}}
	svc := service.NewUserService(repo)
	svc.SetAccountDirectory(&stubDirectory{})
	if _, err := svc.Bootstrap(context.Background(), "alice", models.UserUpdateInput{Email: &email}); !errors.Is(err, domain.ErrEmailInUse) || calls != 1 {
		t.Fatalf("err = %v after %d writes; want ErrEmailInUse after one write", err, calls)
	}
}

// ResolveIdentity tells a lookup failure apart from a directory that has
// nothing more, which the backfill command relies on.
func TestUserService_ResolveIdentity_ReportsLookupFailures(t *testing.T) {
	str := func(v string) *string { return &v }
	for name, tc := range map[string]struct {
		dir     *stubDirectory
		wantErr bool
		wantNm  *string
	}{
		"directory has nothing": {dir: &stubDirectory{}},
		"directory unreachable": {dir: &stubDirectory{err: errors.New("nats: no responders")}, wantErr: true},
		"directory answers":     {dir: &stubDirectory{accounts: map[string]*models.LFAccount{"alice": {Name: str("Alice")}}}, wantNm: str("Alice")},
	} {
		t.Run(name, func(t *testing.T) {
			svc := service.NewUserService(&stubUserRepository{})
			svc.SetAccountDirectory(tc.dir)
			got, err := svc.ResolveIdentity(context.Background(), "alice", models.UserUpdateInput{})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v; wantErr %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got.Name, tc.wantNm) {
				t.Errorf("name = %v; want %v", got.Name, tc.wantNm)
			}
		})
	}
}

func TestUserService_Bootstrap_WithoutDirectoryStoresTokenOnly(t *testing.T) {
	var got models.UserCreateInput
	var gotFill models.UserIdentityFill
	svc := service.NewUserService(&stubUserRepository{bootstrap: func(_ context.Context, in models.UserCreateInput, fill models.UserIdentityFill) (*models.User, error) {
		got, gotFill = in, fill
		return &models.User{LFID: in.LFID}, nil
	}})
	email := "tok@example.org"
	if _, err := svc.Bootstrap(context.Background(), "alice", models.UserUpdateInput{Email: &email}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	lfid := "alice"
	if want := (models.UserCreateInput{LFID: &lfid, Email: &email}); !reflect.DeepEqual(got, want) || !gotFill.IsZero() {
		t.Fatalf("stored %+v fill %+v; want lfid and token email only, no fill", got, gotFill)
	}
}
