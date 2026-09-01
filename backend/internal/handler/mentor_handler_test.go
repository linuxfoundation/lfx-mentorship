// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

type stubMentorSvc struct {
	list        func(context.Context, models.MentorFilter) (*models.MentorPage, error)
	summary     func(context.Context) (*models.MentorSummary, error)
	getByUserID func(context.Context, string) (*models.MentorDetail, error)
}

func (s *stubMentorSvc) List(ctx context.Context, f models.MentorFilter) (*models.MentorPage, error) {
	if s.list != nil {
		return s.list(ctx, f)
	}
	return &models.MentorPage{Data: []*models.MentorItem{}}, nil
}

func (s *stubMentorSvc) Summary(ctx context.Context) (*models.MentorSummary, error) {
	if s.summary != nil {
		return s.summary(ctx)
	}
	return &models.MentorSummary{}, nil
}

func (s *stubMentorSvc) GetByUserID(ctx context.Context, id string) (*models.MentorDetail, error) {
	if s.getByUserID != nil {
		return s.getByUserID(ctx, id)
	}
	return &models.MentorDetail{MentorItem: models.MentorItem{UserID: id}}, nil
}

func TestMentorHandler_List_OK(t *testing.T) {
	var captured models.MentorFilter
	h := handler.NewMentorHandler(&stubMentorSvc{
		list: func(_ context.Context, f models.MentorFilter) (*models.MentorPage, error) {
			captured = f
			name := "Alex"
			return &models.MentorPage{
				Data: []*models.MentorItem{{
					UserID: "u1",
					Name:   &name,
					Skills: []string{"Go"},
				}},
				Meta: models.PaginationMeta{Total: 1, Limit: 20, Offset: 0},
			}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentors?search=alex&skill=Go&limit=20&offset=0", nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if captured.Search != "alex" || captured.Skill != "Go" || captured.Limit != 20 {
		t.Errorf("filter = %+v", captured)
	}
	var body models.MentorPage
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].UserID != "u1" || body.Meta.Total != 1 {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestMentorHandler_Summary_OK(t *testing.T) {
	h := handler.NewMentorHandler(&stubMentorSvc{
		summary: func(context.Context) (*models.MentorSummary, error) {
			return &models.MentorSummary{MentorCount: 8, ProgramCount: 7}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentors/summary", nil)
	w := httptest.NewRecorder()
	h.Summary(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var body models.MentorSummary
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.MentorCount != 8 || body.ProgramCount != 7 {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestMentorHandler_List_InvalidLimit(t *testing.T) {
	h := handler.NewMentorHandler(&stubMentorSvc{})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentors?limit=abc", nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d; want 400", w.Code)
	}
}

func TestMentorHandler_GetByID_OK(t *testing.T) {
	h := handler.NewMentorHandler(&stubMentorSvc{
		getByUserID: func(_ context.Context, id string) (*models.MentorDetail, error) {
			if id != "u1" {
				t.Errorf("id = %q; want u1", id)
			}
			return &models.MentorDetail{
				MentorItem: models.MentorItem{UserID: id, JoinedAt: time.Unix(0, 0).UTC()},
				Programs:   []models.MentorProgram{},
			}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentors/u1", nil)
	r = requestWithChiParam(r, "id", "u1")
	w := httptest.NewRecorder()
	h.GetByID(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var body models.MentorDetail
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.UserID != "u1" {
		t.Errorf("user_id = %q; want u1", body.UserID)
	}
}

func TestMentorHandler_GetByID_NotFound(t *testing.T) {
	h := handler.NewMentorHandler(&stubMentorSvc{
		getByUserID: func(context.Context, string) (*models.MentorDetail, error) {
			return nil, domain.ErrMentorNotFound
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentors/missing", nil)
	r = requestWithChiParam(r, "id", "missing")
	w := httptest.NewRecorder()
	h.GetByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}
