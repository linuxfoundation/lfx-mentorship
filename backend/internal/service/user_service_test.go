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
	bootstrap func(context.Context, models.UserCreateInput) (*models.User, error)
}

func (s *stubUserRepository) GetByID(context.Context, string) (*models.User, error) {
	return &models.User{}, nil
}

func (s *stubUserRepository) GetByLFID(context.Context, string) (*models.User, error) {
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

func (s *stubUserRepository) BootstrapByLFID(ctx context.Context, input models.UserCreateInput) (*models.User, error) {
	if s.bootstrap != nil {
		return s.bootstrap(ctx, input)
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

// Bootstrap fills from the LF directory only what the token left out, as
// lfx-crowdfunding's sign-in sync does, and never fails the sign-in on a
// directory problem (#360).
func TestUserService_Bootstrap_FillsMissingIdentityFromDirectory(t *testing.T) {
	str := func(v string) *string { return &v }
	fullAccount := &models.LFAccount{Username: "alice", Name: str("Alice Liddell"), GivenName: str("Alice"), FamilyName: str("Liddell"), AvatarURL: str("https://img/alice.png")}
	for name, tc := range map[string]struct {
		dir       *stubDirectory
		input     models.UserUpdateInput
		want      models.UserCreateInput
		wantCalls int32
	}{
		"token carried everything: no directory call": {
			dir:   &stubDirectory{accounts: map[string]*models.LFAccount{"alice": fullAccount}, emails: map[string]string{"alice": "dir@example.org"}},
			input: models.UserUpdateInput{Email: str("tok@example.org"), Name: str("Tok Name"), GivenName: str("Tok"), FamilyName: str("Name"), AvatarURL: str("https://img/tok.png")},
			want:  models.UserCreateInput{LFID: str("alice"), Email: str("tok@example.org"), Name: str("Tok Name"), GivenName: str("Tok"), FamilyName: str("Name"), AvatarURL: str("https://img/tok.png")},
		},
		"empty token: directory fills name and email": {
			dir:       &stubDirectory{accounts: map[string]*models.LFAccount{"alice": fullAccount}, emails: map[string]string{"alice": "dir@example.org"}},
			want:      models.UserCreateInput{LFID: str("alice"), Email: str("dir@example.org"), Name: str("Alice Liddell"), GivenName: str("Alice"), FamilyName: str("Liddell"), AvatarURL: str("https://img/alice.png")},
			wantCalls: 2,
		},
		"token had email and name: directory fills only the rest": {
			dir:       &stubDirectory{accounts: map[string]*models.LFAccount{"alice": fullAccount}, emails: map[string]string{"alice": "dir@example.org"}},
			input:     models.UserUpdateInput{Email: str("tok@example.org"), Name: str("Tok Name")},
			want:      models.UserCreateInput{LFID: str("alice"), Email: str("tok@example.org"), Name: str("Tok Name"), GivenName: str("Alice"), FamilyName: str("Liddell"), AvatarURL: str("https://img/alice.png")},
			wantCalls: 1,
		},
		"directory down: sign-in still succeeds with what the token had": {
			dir:       &stubDirectory{err: errors.New("nats: no responders")},
			input:     models.UserUpdateInput{Email: str("tok@example.org")},
			want:      models.UserCreateInput{LFID: str("alice"), Email: str("tok@example.org")},
			wantCalls: 1,
		},
		"no LF account (M2M client): nothing filled, no error": {
			dir:       &stubDirectory{},
			want:      models.UserCreateInput{LFID: str("alice")},
			wantCalls: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var got models.UserCreateInput
			svc := service.NewUserService(&stubUserRepository{bootstrap: func(_ context.Context, in models.UserCreateInput) (*models.User, error) {
				got = in
				return &models.User{LFID: in.LFID}, nil
			}})
			svc.SetAccountDirectory(tc.dir)

			if _, err := svc.Bootstrap(context.Background(), "alice", tc.input); err != nil {
				t.Fatalf("Bootstrap: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				g, _ := json.Marshal(got)
				w, _ := json.Marshal(tc.want)
				t.Fatalf("stored %s; want %s", g, w)
			}
			if calls := tc.dir.calls.Load(); calls != tc.wantCalls {
				t.Errorf("directory calls = %d; want %d", calls, tc.wantCalls)
			}
		})
	}
}

func TestUserService_Bootstrap_WithoutDirectoryStoresTokenOnly(t *testing.T) {
	var got models.UserCreateInput
	svc := service.NewUserService(&stubUserRepository{bootstrap: func(_ context.Context, in models.UserCreateInput) (*models.User, error) {
		got = in
		return &models.User{LFID: in.LFID}, nil
	}})
	email := "tok@example.org"
	if _, err := svc.Bootstrap(context.Background(), "alice", models.UserUpdateInput{Email: &email}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	lfid := "alice"
	if want := (models.UserCreateInput{LFID: &lfid, Email: &email}); !reflect.DeepEqual(got, want) {
		t.Fatalf("stored %+v; want lfid and token email only", got)
	}
}
