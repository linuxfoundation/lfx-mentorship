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

type stubMentorRepo struct {
	list        func(context.Context, models.MentorFilter) (*models.MentorPage, error)
	summary     func(context.Context) (*models.MentorSummary, error)
	getByUserID func(context.Context, string) (*models.MentorDetail, error)
	listMine    func(context.Context, string, models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error)
}

func (s *stubMentorRepo) ListMentoredByUser(ctx context.Context, userID string, f models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error) {
	if s.listMine != nil {
		return s.listMine(ctx, userID, f)
	}
	return []*models.MentoredProgram{}, &models.PaginationMeta{}, nil
}

func (s *stubMentorRepo) List(ctx context.Context, f models.MentorFilter) (*models.MentorPage, error) {
	if s.list != nil {
		return s.list(ctx, f)
	}
	return &models.MentorPage{Data: []*models.MentorItem{}}, nil
}

func (s *stubMentorRepo) Summary(ctx context.Context) (*models.MentorSummary, error) {
	if s.summary != nil {
		return s.summary(ctx)
	}
	return &models.MentorSummary{}, nil
}

func (s *stubMentorRepo) GetByUserID(ctx context.Context, id string) (*models.MentorDetail, error) {
	if s.getByUserID != nil {
		return s.getByUserID(ctx, id)
	}
	return &models.MentorDetail{MentorItem: models.MentorItem{UserID: id}}, nil
}

func TestMentorService_List_NormalizesFilter(t *testing.T) {
	var captured models.MentorFilter
	svc := service.NewMentorService(&stubMentorRepo{
		list: func(_ context.Context, f models.MentorFilter) (*models.MentorPage, error) {
			captured = f
			return &models.MentorPage{Data: []*models.MentorItem{{UserID: "u1"}}}, nil
		},
	})
	page, err := svc.List(context.Background(), models.MentorFilter{
		Search: "  alex  ",
		Skill:  "all",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if captured.Search != "alex" || captured.Skill != "" {
		t.Errorf("filter = %+v; want trimmed search and cleared skill", captured)
	}
	if len(page.Data) != 1 || page.Data[0].Skills == nil {
		t.Errorf("expected empty slices on list item, got %+v", page.Data[0])
	}
}

func TestMentorService_List_KeepsSkill(t *testing.T) {
	var captured models.MentorFilter
	svc := service.NewMentorService(&stubMentorRepo{
		list: func(_ context.Context, f models.MentorFilter) (*models.MentorPage, error) {
			captured = f
			return &models.MentorPage{}, nil
		},
	})
	if _, err := svc.List(context.Background(), models.MentorFilter{Skill: "Go"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if captured.Skill != "Go" {
		t.Errorf("filter = %+v; want skill=Go", captured)
	}
}

func TestMentorService_Summary(t *testing.T) {
	svc := service.NewMentorService(&stubMentorRepo{
		summary: func(context.Context) (*models.MentorSummary, error) {
			return &models.MentorSummary{MentorCount: 8, ProgramCount: 7}, nil
		},
	})
	summary, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.MentorCount != 8 || summary.ProgramCount != 7 {
		t.Errorf("summary = %+v", summary)
	}
}

func TestMentorService_GetByUserID_EmptyID(t *testing.T) {
	svc := service.NewMentorService(&stubMentorRepo{})
	_, err := svc.GetByUserID(context.Background(), "  ")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("got %v; want ErrInvalidInput", err)
	}
}

func TestMentorService_GetByUserID_InvalidUUID(t *testing.T) {
	called := false
	svc := service.NewMentorService(&stubMentorRepo{
		getByUserID: func(context.Context, string) (*models.MentorDetail, error) {
			called = true
			return nil, domain.ErrMentorNotFound
		},
	})
	_, err := svc.GetByUserID(context.Background(), "not-a-uuid")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("got %v; want ErrInvalidInput", err)
	}
	if called {
		t.Error("repo.GetByUserID should not be called for a malformed id")
	}
}

func TestMentorService_GetByUserID_NotFound(t *testing.T) {
	svc := service.NewMentorService(&stubMentorRepo{
		getByUserID: func(context.Context, string) (*models.MentorDetail, error) {
			return nil, domain.ErrMentorNotFound
		},
	})
	_, err := svc.GetByUserID(context.Background(), "455f4f53-fe87-4c99-a174-9f86ccdcf0be")
	if !errors.Is(err, domain.ErrMentorNotFound) {
		t.Errorf("got %v; want ErrMentorNotFound", err)
	}
}

func TestMentorService_GetByUserID_FillsEmptySlices(t *testing.T) {
	svc := service.NewMentorService(&stubMentorRepo{
		getByUserID: func(_ context.Context, id string) (*models.MentorDetail, error) {
			return &models.MentorDetail{
				MentorItem: models.MentorItem{UserID: id},
				Programs:   []models.MentorProgram{{ID: "p1", Name: "K8s"}},
			}, nil
		},
	})
	detail, err := svc.GetByUserID(context.Background(), "455f4f53-fe87-4c99-a174-9f86ccdcf0be")
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if detail.Skills == nil || detail.CurrentMentees == nil || detail.GraduatedMentees == nil ||
		detail.Programs[0].Skills == nil || detail.Programs[0].Mentors == nil {
		t.Errorf("expected empty slices, got %+v", detail)
	}
}

func TestMentorService_ListMine_ScopesToCaller(t *testing.T) {
	var gotUser string
	var gotFilter models.MentoredProgramFilter
	svc := service.NewMentorService(&stubMentorRepo{
		listMine: func(_ context.Context, userID string, f models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error) {
			gotUser, gotFilter = userID, f
			return []*models.MentoredProgram{{ID: "p1"}}, &models.PaginationMeta{Total: 1, Limit: 5, Offset: 10}, nil
		},
	})
	got, meta, err := svc.ListMine(context.Background(), "user-1", models.MentoredProgramFilter{Limit: 5, Offset: 10})
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	if gotUser != "user-1" || gotFilter != (models.MentoredProgramFilter{Limit: 5, Offset: 10}) {
		t.Errorf("repo called with user %q filter %+v; want user-1 and the caller's paging", gotUser, gotFilter)
	}
	if len(got) != 1 || got[0].ID != "p1" || meta.Total != 1 {
		t.Errorf("got %+v meta %+v; want the repository's page", got, meta)
	}
}

func TestMentorService_ListMine_RequiresCaller(t *testing.T) {
	svc := service.NewMentorService(&stubMentorRepo{
		listMine: func(context.Context, string, models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error) {
			t.Fatal("repository must not be called without a caller")
			return nil, nil, nil
		},
	})
	if _, _, err := svc.ListMine(context.Background(), "", models.MentoredProgramFilter{}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("got %v; want ErrUnauthorized", err)
	}
}

func TestMentorService_ListMine_WrapsRepositoryError(t *testing.T) {
	boom := errors.New("boom")
	svc := service.NewMentorService(&stubMentorRepo{
		listMine: func(context.Context, string, models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error) {
			return nil, nil, boom
		},
	})
	if _, _, err := svc.ListMine(context.Background(), "user-1", models.MentoredProgramFilter{}); !errors.Is(err, boom) {
		t.Errorf("got %v; want the repository error wrapped", err)
	}
}
