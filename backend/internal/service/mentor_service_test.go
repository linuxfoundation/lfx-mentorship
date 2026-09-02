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
