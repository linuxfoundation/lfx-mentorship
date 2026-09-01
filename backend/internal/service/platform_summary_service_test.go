// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

type stubPlatformSummaryRepo struct {
	summary func(context.Context) (*models.PlatformSummary, error)
}

func (s *stubPlatformSummaryRepo) Summary(ctx context.Context) (*models.PlatformSummary, error) {
	if s.summary != nil {
		return s.summary(ctx)
	}
	return &models.PlatformSummary{}, nil
}

func TestPlatformSummaryService_Summary(t *testing.T) {
	john, jane := "John Doe", "Jane Doe"
	avatar := "https://example.com/avatar.jpg"
	svc := service.NewPlatformSummaryService(&stubPlatformSummaryRepo{
		summary: func(context.Context) (*models.PlatformSummary, error) {
			return &models.PlatformSummary{
				ProgramCount:          18,
				AcceptingProgramCount: 3,
				MentorCount:           7,
				GraduatedMenteeCount:  42,
				GraduatedMenteeUsers: []models.PlatformSummaryMentee{
					{Name: &john, AvatarURL: &avatar},
					{Name: &jane, AvatarURL: &avatar},
				},
			}, nil
		},
	})
	got, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	want := models.PlatformSummary{
		ProgramCount:          18,
		AcceptingProgramCount: 3,
		MentorCount:           7,
		GraduatedMenteeCount:  42,
		GraduatedMenteeUsers: []models.PlatformSummaryMentee{
			{Name: &john, AvatarURL: &avatar},
			{Name: &jane, AvatarURL: &avatar},
		},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("summary = %+v; want %+v", *got, want)
	}
}

func TestPlatformSummaryService_Summary_NilPreview(t *testing.T) {
	svc := service.NewPlatformSummaryService(&stubPlatformSummaryRepo{
		summary: func(context.Context) (*models.PlatformSummary, error) {
			return &models.PlatformSummary{ProgramCount: 1}, nil
		},
	})
	got, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.GraduatedMenteeUsers == nil {
		t.Error("expected empty graduated mentee slice, got nil")
	}
}

func TestPlatformSummaryService_Summary_RepoError(t *testing.T) {
	sentinel := errors.New("boom")
	svc := service.NewPlatformSummaryService(&stubPlatformSummaryRepo{
		summary: func(context.Context) (*models.PlatformSummary, error) {
			return nil, sentinel
		},
	})
	_, err := svc.Summary(context.Background())
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v; want wrapped sentinel", err)
	}
}
