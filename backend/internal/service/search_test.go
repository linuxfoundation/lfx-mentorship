// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

// The limits count characters, so multibyte runes sit exactly on them.
var (
	longestSearch = strings.Repeat("é", 128)
	longestName   = strings.Repeat("é", 255)
)

func TestMenteeService_List_SearchLength(t *testing.T) {
	var captured models.MenteeFilter
	called := false
	svc := service.NewMenteeService(&stubMenteeRepo{
		list: func(_ context.Context, f models.MenteeFilter) (*models.MenteePage, error) {
			called = true
			captured = f
			return &models.MenteePage{}, nil
		},
	})
	if _, err := svc.List(context.Background(), models.MenteeFilter{Search: " " + longestSearch + " "}); err != nil {
		t.Fatalf("List(128-character search): %v", err)
	}
	if captured.Search != longestSearch {
		t.Errorf("search = %q; want it trimmed", captured.Search)
	}

	called = false
	_, err := svc.List(context.Background(), models.MenteeFilter{Search: longestSearch + "é"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("List(129-character search) err = %v; want ErrInvalidInput", err)
	}
	if called {
		t.Error("repo.List should not be called for an over-long search")
	}
}

func TestMentorService_List_RejectsLongSearch(t *testing.T) {
	called := false
	svc := service.NewMentorService(&stubMentorRepo{
		list: func(context.Context, models.MentorFilter) (*models.MentorPage, error) {
			called = true
			return &models.MentorPage{}, nil
		},
	})
	_, err := svc.List(context.Background(), models.MentorFilter{Search: longestSearch + "é"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("err = %v; want ErrInvalidInput", err)
	}
	if called {
		t.Error("repo.List should not be called for an over-long search")
	}
}

func TestProgramService_ListCatalog_RejectsLongSearch(t *testing.T) {
	called := false
	svc := newProgramSvc(&stubProgRepo{
		listCatalog: func(context.Context, models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error) {
			called = true
			return []*models.ProgramCatalogItem{}, &models.PaginationMeta{}, nil
		},
	}, &stubTermRepo{}, &stubAppRepo{})
	_, _, err := svc.ListCatalog(context.Background(), models.ProgramFilter{Search: longestSearch + "é"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("err = %v; want ErrInvalidInput", err)
	}
	if called {
		t.Error("repo.ListCatalog should not be called for an over-long search")
	}
}

// Every list that takes a search applies the cap; the stub repositories would otherwise succeed.
func TestListEntryPoints_RejectLongSearch(t *testing.T) {
	ctx := context.Background()
	search := longestSearch + "é"
	programs := newProgramSvc(&stubProgRepo{}, &stubTermRepo{}, &stubAppRepo{})
	members := service.NewProgramMemberService(&stubMemberRepo{}, publishedProgRepo(), &stubInviteeUsers{}, nil, &stubNotifier{}, "test-secret")
	for name, list := range map[string]func() error{
		"ProgramService.List": func() error {
			_, _, err := programs.List(ctx, models.ProgramFilter{Search: search})
			return err
		},
		"ProgramService.ListMine": func() error {
			_, _, err := programs.ListMine(ctx, "user-1", models.AdministeredProgramFilter{Search: search})
			return err
		},
		"ApplicationService.ListByProgram": func() error {
			_, _, err := newApplicationSvc(&stubAppRepo{}, &stubTaskRepo{}, &stubTermRepo{}, &stubProgRepo{}).
				ListByProgram(ctx, "prog-1", models.ProgramApplicationFilter{Search: search})
			return err
		},
		"ProgramMemberService.ListMentorManagement": func() error {
			_, _, err := members.ListMentorManagement(ctx, "prog-1", models.ProgramMemberFilter{Search: search})
			return err
		},
		"ProgramMemberService.SearchCandidates": func() error {
			_, err := members.SearchCandidates(ctx, "prog-1", search)
			return err
		},
		"UserService.List": func() error {
			_, _, err := service.NewUserService(&stubUserRepository{}).List(ctx, models.UserFilter{Search: search})
			return err
		},
	} {
		if err := list(); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s(129-character search) err = %v; want ErrInvalidInput", name, err)
		}
	}
}

func TestProgramMemberService_Create_DropsLongDirectoryName(t *testing.T) {
	long := longestName + "é"
	dir := &stubDirectory{
		accounts: map[string]*models.LFAccount{"ada": {Username: "ada", Name: &long, GivenName: ptr("Ada"), FamilyName: &long}},
		emails:   map[string]string{"ada": "ada@example.org"},
	}
	users := &stubInviteeUsers{}
	var captured models.ProgramMemberCreateInput
	if _, err := newInviteSvc(users, dir, capturingMemberRepo(&captured), &stubNotifier{}).
		Create(context.Background(), "prog-1", models.ProgramMemberCreateInput{LFID: "ada", MemberType: models.MemberTypeMentor}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if users.upserted == nil || users.upserted.Name != nil || users.upserted.FamilyName != nil || users.upserted.GivenName == nil || *users.upserted.GivenName != "Ada" {
		t.Errorf("upserted = %+v; want the over-long names left out and the given name kept", users.upserted)
	}
}

func TestUserService_Update_NameLength(t *testing.T) {
	called := false
	svc := service.NewUserService(&stubUserRepository{
		update: func(context.Context, string, models.UserUpdateInput) (*models.User, error) {
			called = true
			return &models.User{}, nil
		},
	})
	name := longestName
	if _, err := svc.Update(context.Background(), "user-1", models.UserUpdateInput{Name: &name}); err != nil {
		t.Fatalf("Update(255-character name): %v", err)
	}

	tooLong := longestName + "é"
	for field, input := range map[string]models.UserUpdateInput{
		"name":        {Name: &tooLong},
		"given_name":  {GivenName: &tooLong},
		"family_name": {FamilyName: &tooLong},
	} {
		called = false
		_, err := svc.Update(context.Background(), "user-1", input)
		if !errors.Is(err, domain.ErrInvalidInput) || !strings.Contains(err.Error(), field) {
			t.Errorf("Update(long %s) err = %v; want ErrInvalidInput naming the field", field, err)
		}
		if called {
			t.Errorf("repository update should not be called for a long %s", field)
		}
	}
}

func TestUserService_Bootstrap_RejectsLongName(t *testing.T) {
	called := false
	svc := service.NewUserService(&stubUserRepository{
		bootstrap: func(context.Context, models.UserCreateInput, models.UserIdentityFill) (*models.User, error) {
			called = true
			return &models.User{}, nil
		},
	})
	name := longestName + "é"
	_, err := svc.Bootstrap(context.Background(), "lfid", models.UserUpdateInput{Name: &name})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("err = %v; want ErrInvalidInput", err)
	}
	if called {
		t.Error("repository bootstrap should not be called for a long name")
	}
}

func TestUserProfileService_Update_RejectsLongName(t *testing.T) {
	called := false
	svc := newUserProfileSvc(&stubUserProfileRepo{
		update: func(context.Context, string, models.UserProfileUpdateInput) (*models.UserProfile, error) {
			called = true
			return &models.UserProfile{}, nil
		},
	})
	name := longestName + "é"
	_, err := svc.Update(context.Background(), "profile-1", models.UserProfileUpdateInput{LastName: &name})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("err = %v; want ErrInvalidInput", err)
	}
	if called {
		t.Error("repository update should not be called for a long last name")
	}
}

func TestProgramService_Update_RejectsLongName(t *testing.T) {
	called := false
	svc := newProgramSvc(&stubProgRepo{
		update: func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error) {
			called = true
			return &models.Program{}, nil
		},
	}, &stubTermRepo{}, &stubAppRepo{})
	name := longestName + "é"
	_, err := svc.Update(context.Background(), "program-1", models.ProgramUpdateInput{Name: &name})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("err = %v; want ErrInvalidInput", err)
	}
	if called {
		t.Error("repository update should not be called for a long name")
	}
}
