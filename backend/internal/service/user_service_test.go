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
	upsert    func(context.Context, models.UserCreateInput) (*models.User, error)
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

func (s *stubUserRepository) UpsertByLFID(ctx context.Context, input models.UserCreateInput) (*models.User, error) {
	if s.upsert != nil {
		return s.upsert(ctx, input)
	}
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

// Bootstrap refreshes name and avatar from the LF directory on every sign-in:
// the directory's values replace stored ones, falling back to the token's
// where the directory has none. A token email is written through
// BootstrapByLFID, which may reclaim it; without one the directory's primary
// email replaces the stored one through UpsertByLFID, which never takes it
// from another row. A directory problem never fails the sign-in (#360).
func TestUserService_Bootstrap_RefreshesProfileFromDirectory(t *testing.T) {
	str := func(v string) *string { return &v }
	fullAccount := &models.LFAccount{Username: "alice", Name: str("Alice Liddell"), GivenName: str("Alice"), FamilyName: str("Liddell"), AvatarURL: str("https://img/alice.png")}
	knownDir := func() *stubDirectory {
		return &stubDirectory{accounts: map[string]*models.LFAccount{"alice": fullAccount}, emails: map[string]string{"alice": "dir@example.org"}}
	}
	dirProfile := func(email *string) models.UserCreateInput {
		return models.UserCreateInput{LFID: str("alice"), Email: email, Name: str("Alice Liddell"), GivenName: str("Alice"), FamilyName: str("Liddell"), AvatarURL: str("https://img/alice.png")}
	}
	const viaBootstrap, viaUpsert = "BootstrapByLFID", "UpsertByLFID"
	for name, tc := range map[string]struct {
		dir       *stubDirectory
		input     models.UserUpdateInput
		wantInput models.UserCreateInput
		wantVia   string
		wantCalls int32
	}{
		"token carries a stale profile: the directory's replaces it, the token email is kept": {
			dir:       knownDir(),
			input:     models.UserUpdateInput{Email: str("tok@example.org"), Name: str("Tok Name"), GivenName: str("Tok"), FamilyName: str("Name"), AvatarURL: str("https://img/tok.png")},
			wantInput: dirProfile(str("tok@example.org")),
			wantVia:   viaBootstrap,
			wantCalls: 1,
		},
		"no token email: the directory supplies the profile and replaces the stored email": {
			dir:       knownDir(),
			wantInput: dirProfile(str("dir@example.org")),
			wantVia:   viaUpsert,
			wantCalls: 2,
		},
		"directory lacks the avatar: the token's is kept": {
			dir: &stubDirectory{accounts: map[string]*models.LFAccount{"alice": {
				Username: "alice", Name: str("Alice Liddell"), GivenName: str("Alice"), FamilyName: str("Liddell"),
			}}},
			input:     models.UserUpdateInput{Email: str("tok@example.org"), AvatarURL: str("https://img/tok.png")},
			wantInput: models.UserCreateInput{LFID: str("alice"), Email: str("tok@example.org"), Name: str("Alice Liddell"), GivenName: str("Alice"), FamilyName: str("Liddell"), AvatarURL: str("https://img/tok.png")},
			wantVia:   viaBootstrap,
			wantCalls: 1,
		},
		"directory name over the length cap: left out, the rest refreshed": {
			dir: &stubDirectory{accounts: map[string]*models.LFAccount{"alice": {
				Username: "alice", Name: str(longestName + "é"), GivenName: str("Alice"), FamilyName: str(longestName + "é"),
			}}, emails: map[string]string{"alice": "dir@example.org"}},
			wantInput: models.UserCreateInput{LFID: str("alice"), Email: str("dir@example.org"), GivenName: str("Alice")},
			wantVia:   viaUpsert,
			wantCalls: 2,
		},
		"directory down: sign-in still succeeds with what the token had": {
			dir:       &stubDirectory{err: errors.New("nats: no responders")},
			input:     models.UserUpdateInput{Email: str("tok@example.org"), Name: str("Tok Name")},
			wantInput: models.UserCreateInput{LFID: str("alice"), Email: str("tok@example.org"), Name: str("Tok Name")},
			wantVia:   viaBootstrap,
			wantCalls: 1,
		},
		"directory down, no token email: the stored email is left as is": {
			dir:       &stubDirectory{err: errors.New("nats: no responders")},
			wantInput: models.UserCreateInput{LFID: str("alice")},
			wantVia:   viaUpsert,
			wantCalls: 2,
		},
		"no LF account (M2M client): nothing refreshed, no error": {
			dir:       &stubDirectory{},
			wantInput: models.UserCreateInput{LFID: str("alice")},
			wantVia:   viaUpsert,
			wantCalls: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var gotInput models.UserCreateInput
			var gotVia []string
			record := func(via string) func(context.Context, models.UserCreateInput) (*models.User, error) {
				return func(_ context.Context, in models.UserCreateInput) (*models.User, error) {
					gotInput, gotVia = in, append(gotVia, via)
					return &models.User{LFID: in.LFID}, nil
				}
			}
			svc := service.NewUserService(&stubUserRepository{bootstrap: record(viaBootstrap), upsert: record(viaUpsert)})
			svc.SetAccountDirectory(tc.dir)

			if _, err := svc.Bootstrap(context.Background(), "alice", tc.input); err != nil {
				t.Fatalf("Bootstrap: %v", err)
			}
			if !reflect.DeepEqual(gotInput, tc.wantInput) {
				g, _ := json.Marshal(gotInput)
				w, _ := json.Marshal(tc.wantInput)
				t.Fatalf("input %s; want %s", g, w)
			}
			if want := []string{tc.wantVia}; !reflect.DeepEqual(gotVia, want) {
				t.Errorf("writes = %v; want %v", gotVia, want)
			}
			if calls := tc.dir.calls.Load(); calls != tc.wantCalls {
				t.Errorf("directory calls = %d; want %d", calls, tc.wantCalls)
			}
		})
	}
}

// A directory email another row holds is given up, not taken: the directory
// cannot vouch that the address is verified, so the stored email is kept.
func TestUserService_Bootstrap_DirectoryEmailYieldsOnConflict(t *testing.T) {
	var emails []*string
	repo := &stubUserRepository{upsert: func(_ context.Context, in models.UserCreateInput) (*models.User, error) {
		emails = append(emails, in.Email)
		if in.Email != nil {
			return nil, domain.ErrEmailInUse
		}
		return &models.User{LFID: in.LFID}, nil
	}}
	svc := service.NewUserService(repo)
	svc.SetAccountDirectory(&stubDirectory{emails: map[string]string{"alice": "shared@example.org"}})

	if _, err := svc.Bootstrap(context.Background(), "alice", models.UserUpdateInput{}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if len(emails) != 2 || emails[0] == nil || *emails[0] != "shared@example.org" || emails[1] != nil {
		t.Fatalf("emails = %v; want one write with the directory email, then one without", emails)
	}
}

// A conflict on a token email is not retried: it was released first, so it is a real error.
func TestUserService_Bootstrap_TokenEmailConflictIsAnError(t *testing.T) {
	email := "tok@example.org"
	calls := 0
	repo := &stubUserRepository{bootstrap: func(context.Context, models.UserCreateInput) (*models.User, error) {
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
