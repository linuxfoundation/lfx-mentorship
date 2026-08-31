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

type stubMenteeSvc struct {
	list        func(context.Context, models.MenteeFilter) (*models.MenteePage, error)
	summary     func(context.Context) (*models.MenteeSummary, error)
	getByUserID func(context.Context, string) (*models.MenteeDetail, error)
}

func (s *stubMenteeSvc) List(ctx context.Context, f models.MenteeFilter) (*models.MenteePage, error) {
	if s.list != nil {
		return s.list(ctx, f)
	}
	return &models.MenteePage{Data: []*models.MenteeItem{}}, nil
}

func (s *stubMenteeSvc) Summary(ctx context.Context) (*models.MenteeSummary, error) {
	if s.summary != nil {
		return s.summary(ctx)
	}
	return &models.MenteeSummary{}, nil
}

func (s *stubMenteeSvc) GetByUserID(ctx context.Context, id string) (*models.MenteeDetail, error) {
	if s.getByUserID != nil {
		return s.getByUserID(ctx, id)
	}
	return &models.MenteeDetail{MenteeItem: models.MenteeItem{UserID: id}}, nil
}

func TestMenteeHandler_List_OK(t *testing.T) {
	var captured models.MenteeFilter
	h := handler.NewMenteeHandler(&stubMenteeSvc{
		list: func(_ context.Context, f models.MenteeFilter) (*models.MenteePage, error) {
			captured = f
			name := "Alex"
			return &models.MenteePage{
				Data: []*models.MenteeItem{{
					UserID: "u1",
					Name:   &name,
					Status: "active",
					Skills: []string{"Go"},
				}},
				Meta: models.PaginationMeta{Total: 1, Limit: 20, Offset: 0},
			}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentees?search=alex&skill=Go&status=active&limit=20&offset=0", nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if captured.Search != "alex" || captured.Skill != "Go" || captured.Status != "active" || captured.Limit != 20 {
		t.Errorf("filter = %+v", captured)
	}
	var body models.MenteePage
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].UserID != "u1" || body.Meta.Total != 1 {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestMenteeHandler_Summary_OK(t *testing.T) {
	h := handler.NewMenteeHandler(&stubMenteeSvc{
		summary: func(context.Context) (*models.MenteeSummary, error) {
			return &models.MenteeSummary{MenteeCount: 18, ProgramCount: 7}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentees/summary", nil)
	w := httptest.NewRecorder()
	h.Summary(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var body models.MenteeSummary
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.MenteeCount != 18 || body.ProgramCount != 7 {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestMenteeHandler_List_InvalidLimit(t *testing.T) {
	h := handler.NewMenteeHandler(&stubMenteeSvc{})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentees?limit=abc", nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d; want 400", w.Code)
	}
}

func TestMenteeHandler_GetByID_OK(t *testing.T) {
	h := handler.NewMenteeHandler(&stubMenteeSvc{
		getByUserID: func(_ context.Context, id string) (*models.MenteeDetail, error) {
			if id != "u1" {
				t.Errorf("id = %q; want u1", id)
			}
			return &models.MenteeDetail{
				MenteeItem: models.MenteeItem{UserID: id, Status: "active", JoinedAt: time.Unix(0, 0).UTC()},
				Programs:   []models.MenteeProgram{},
			}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentees/u1", nil)
	r = requestWithChiParam(r, "id", "u1")
	w := httptest.NewRecorder()
	h.GetByID(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var body models.MenteeDetail
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.UserID != "u1" {
		t.Errorf("user_id = %q; want u1", body.UserID)
	}
}

func TestMenteeHandler_GetByID_NotFound(t *testing.T) {
	h := handler.NewMenteeHandler(&stubMenteeSvc{
		getByUserID: func(context.Context, string) (*models.MenteeDetail, error) {
			return nil, domain.ErrMenteeNotFound
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/mentees/missing", nil)
	r = requestWithChiParam(r, "id", "missing")
	w := httptest.NewRecorder()
	h.GetByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}
