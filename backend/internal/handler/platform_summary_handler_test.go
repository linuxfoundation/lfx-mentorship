// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

type stubPlatformSummarySvc struct {
	summary func(context.Context) (*models.PlatformSummary, error)
}

func (s *stubPlatformSummarySvc) Summary(ctx context.Context) (*models.PlatformSummary, error) {
	if s.summary != nil {
		return s.summary(ctx)
	}
	return &models.PlatformSummary{}, nil
}

func TestPlatformSummaryHandler_Get_OK(t *testing.T) {
	john, jane := "John Doe", "Jane Doe"
	avatar := "https://example.com/avatar.jpg"
	h := handler.NewPlatformSummaryHandler(&stubPlatformSummarySvc{
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
	r := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var body models.PlatformSummary
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
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
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body = %+v; want %+v", body, want)
	}
}

func TestPlatformSummaryHandler_Get_ServiceError(t *testing.T) {
	h := handler.NewPlatformSummaryHandler(&stubPlatformSummarySvc{
		summary: func(context.Context) (*models.PlatformSummary, error) {
			return nil, errors.New("boom")
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/summary", nil)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500", w.Code)
	}
}
