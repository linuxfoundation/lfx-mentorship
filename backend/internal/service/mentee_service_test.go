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

type stubMenteeRepo struct {
	list        func(context.Context, models.MenteeFilter) (*models.MenteePage, error)
	summary     func(context.Context) (*models.MenteeSummary, error)
	getByUserID func(context.Context, string) (*models.MenteeDetail, error)
}

func (s *stubMenteeRepo) List(ctx context.Context, f models.MenteeFilter) (*models.MenteePage, error) {
	if s.list != nil {
		return s.list(ctx, f)
	}
	return &models.MenteePage{Data: []*models.MenteeItem{}}, nil
}

func (s *stubMenteeRepo) Summary(ctx context.Context) (*models.MenteeSummary, error) {
	if s.summary != nil {
		return s.summary(ctx)
	}
	return &models.MenteeSummary{}, nil
}

func (s *stubMenteeRepo) GetByUserID(ctx context.Context, id string) (*models.MenteeDetail, error) {
	if s.getByUserID != nil {
		return s.getByUserID(ctx, id)
	}
	return &models.MenteeDetail{MenteeItem: models.MenteeItem{UserID: id}}, nil
}

func TestMenteeService_List_NormalizesFilter(t *testing.T) {
	var captured models.MenteeFilter
	svc := service.NewMenteeService(&stubMenteeRepo{
		list: func(_ context.Context, f models.MenteeFilter) (*models.MenteePage, error) {
			captured = f
			return &models.MenteePage{Data: []*models.MenteeItem{{UserID: "u1"}}}, nil
		},
	})
	page, err := svc.List(context.Background(), models.MenteeFilter{
		Search: "  alex  ",
		Skill:  "all",
		Status: "ALL",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if captured.Search != "alex" || captured.Skill != "" || captured.Status != "" {
		t.Errorf("filter = %+v; want trimmed search and cleared skill/status", captured)
	}
	if len(page.Data) != 1 || page.Data[0].Skills == nil || page.Data[0].Mentors == nil {
		t.Errorf("expected empty slices on list item, got %+v", page.Data[0])
	}
}

func TestMenteeService_List_KeepsActiveStatus(t *testing.T) {
	var captured models.MenteeFilter
	svc := service.NewMenteeService(&stubMenteeRepo{
		list: func(_ context.Context, f models.MenteeFilter) (*models.MenteePage, error) {
			captured = f
			return &models.MenteePage{}, nil
		},
	})
	if _, err := svc.List(context.Background(), models.MenteeFilter{Status: "Active", Skill: "Go"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if captured.Status != "active" || captured.Skill != "Go" {
		t.Errorf("filter = %+v; want status=active skill=Go", captured)
	}
}

func TestMenteeService_Summary(t *testing.T) {
	svc := service.NewMenteeService(&stubMenteeRepo{
		summary: func(context.Context) (*models.MenteeSummary, error) {
			return &models.MenteeSummary{MenteeCount: 18, ProgramCount: 7}, nil
		},
	})
	summary, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.MenteeCount != 18 || summary.ProgramCount != 7 {
		t.Errorf("summary = %+v", summary)
	}
}

func TestMenteeService_GetByUserID_EmptyID(t *testing.T) {
	svc := service.NewMenteeService(&stubMenteeRepo{})
	_, err := svc.GetByUserID(context.Background(), "  ")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("got %v; want ErrInvalidInput", err)
	}
}

func TestMenteeService_GetByUserID_NotFound(t *testing.T) {
	svc := service.NewMenteeService(&stubMenteeRepo{
		getByUserID: func(context.Context, string) (*models.MenteeDetail, error) {
			return nil, domain.ErrMenteeNotFound
		},
	})
	_, err := svc.GetByUserID(context.Background(), "missing")
	if !errors.Is(err, domain.ErrMenteeNotFound) {
		t.Errorf("got %v; want ErrMenteeNotFound", err)
	}
}

func TestMenteeService_GetByUserID_FillsEmptySlices(t *testing.T) {
	svc := service.NewMenteeService(&stubMenteeRepo{
		getByUserID: func(_ context.Context, id string) (*models.MenteeDetail, error) {
			return &models.MenteeDetail{
				MenteeItem: models.MenteeItem{UserID: id},
				Programs: []models.MenteeProgram{{
					ID:   "p1",
					Name: "K8s",
				}},
			}, nil
		},
	})
	detail, err := svc.GetByUserID(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if detail.Skills == nil || detail.Mentors == nil || detail.Programs[0].Skills == nil || detail.Programs[0].Mentors == nil {
		t.Errorf("expected empty slices, got %+v", detail)
	}
}
