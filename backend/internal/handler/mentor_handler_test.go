// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type stubMentorSvc struct {
	list        func(context.Context, models.MentorFilter) (*models.MentorPage, error)
	summary     func(context.Context) (*models.MentorSummary, error)
	getByUserID func(context.Context, string) (*models.MentorDetail, error)
	listMine    func(context.Context, string, models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error)
}

func (s *stubMentorSvc) ListMine(ctx context.Context, userID string, f models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error) {
	if s.listMine != nil {
		return s.listMine(ctx, userID, f)
	}
	return []*models.MentoredProgram{}, &models.PaginationMeta{}, nil
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

func TestMentorHandler_ListMine_ScopesToPrincipal(t *testing.T) {
	var gotUser string
	var gotFilter models.MentoredProgramFilter
	h := handler.NewMentorHandler(&stubMentorSvc{
		listMine: func(_ context.Context, userID string, f models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error) {
			gotUser, gotFilter = userID, f
			return []*models.MentoredProgram{{
				ID:     "p1",
				Name:   "GridFlow",
				Status: models.MentoredProgramStatusOpen,
				Stats:  models.MentoredProgramStats{Mentees: 3, Applicants: 12, TasksToReview: 2},
			}}, &models.PaginationMeta{Total: 1, Limit: 5, Offset: 10}, nil
		},
	})
	r := requestWithPrincipal(httptest.NewRequest(http.MethodGet, "/v1/me/mentor-programs?limit=5&offset=10", nil), "caller-user")
	w := httptest.NewRecorder()
	h.ListMine(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200: %s", w.Code, w.Body.String())
	}
	if gotUser != "caller-user" {
		t.Errorf("user = %q; want caller-user", gotUser)
	}
	if want := (models.MentoredProgramFilter{Limit: 5, Offset: 10}); gotFilter != want {
		t.Errorf("filter = %+v; want %+v", gotFilter, want)
	}
	raw := w.Body.Bytes()
	var body struct {
		Data []models.MentoredProgram `json:"data"`
		Meta models.PaginationMeta    `json:"meta"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 || body.Meta.Total != 1 {
		t.Fatalf("unexpected body: %+v", body)
	}
	got := body.Data[0]
	if got.Status != models.MentoredProgramStatusOpen ||
		got.Stats != (models.MentoredProgramStats{Mentees: 3, Applicants: 12, TasksToReview: 2}) {
		t.Errorf("row = %+v; want an open program with the service's stats", got)
	}
	// Typed decoding ignores unknown fields, so check the row's keys directly.
	var fields struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode fields: %v", err)
	}
	for _, removed := range []string{"term", "term_status"} {
		if _, ok := fields.Data[0][removed]; ok {
			t.Errorf("row has %q; want it removed: %s", removed, raw)
		}
	}
	if _, ok := fields.Data[0]["status"]; !ok {
		t.Errorf("row has no status: %s", raw)
	}
}

func TestMentorHandler_ListMine_EmptyIsDataArray(t *testing.T) {
	h := handler.NewMentorHandler(&stubMentorSvc{})
	r := requestWithPrincipal(httptest.NewRequest(http.MethodGet, "/v1/me/mentor-programs", nil), "caller-user")
	w := httptest.NewRecorder()
	h.ListMine(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Errorf("body = %s; want an empty data array", w.Body.String())
	}
}

func TestMentorHandler_ListMine_RequiresUserPrincipal(t *testing.T) {
	h := handler.NewMentorHandler(&stubMentorSvc{
		listMine: func(context.Context, string, models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error) {
			t.Fatal("service must not be called without a user principal")
			return nil, nil, nil
		},
	})
	for name, principal := range map[string]*models.Principal{
		"none": nil,
		"m2m":  {UserID: "client-id@clients", Username: "client-id@clients"},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/v1/me/mentor-programs", nil)
			if principal != nil {
				r = r.WithContext(auth.ContextWithPrincipal(r.Context(), principal))
			}
			w := httptest.NewRecorder()
			h.ListMine(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("got %d; want 401", w.Code)
			}
		})
	}
}

func TestMentorHandler_ListMine_BadPagination(t *testing.T) {
	h := handler.NewMentorHandler(&stubMentorSvc{
		listMine: func(context.Context, string, models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error) {
			t.Fatal("service must not be called with bad paging")
			return nil, nil, nil
		},
	})
	for _, query := range []string{"limit=abc", "offset=abc"} {
		t.Run(query, func(t *testing.T) {
			r := requestWithPrincipal(httptest.NewRequest(http.MethodGet, "/v1/me/mentor-programs?"+query, nil), "caller-user")
			w := httptest.NewRecorder()
			h.ListMine(w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("got %d; want 400", w.Code)
			}
		})
	}
}

func TestMentorHandler_ListMine_OutOfRangePagination(t *testing.T) {
	for query, want := range map[string]models.MentoredProgramFilter{
		"limit=101": {Limit: 101},
		"offset=-1": {Offset: -1},
	} {
		t.Run(query, func(t *testing.T) {
			var got models.MentoredProgramFilter
			h := handler.NewMentorHandler(&stubMentorSvc{
				listMine: func(_ context.Context, _ string, f models.MentoredProgramFilter) ([]*models.MentoredProgram, *models.PaginationMeta, error) {
					got = f
					return nil, nil, fmt.Errorf("%w: out of range", domain.ErrInvalidInput)
				},
			})
			r := requestWithPrincipal(httptest.NewRequest(http.MethodGet, "/v1/me/mentor-programs?"+query, nil), "caller-user")
			w := httptest.NewRecorder()
			h.ListMine(w, r)
			if got != want {
				t.Errorf("filter = %+v; want %+v passed to the service", got, want)
			}
			if w.Code != http.StatusBadRequest {
				t.Errorf("got %d; want 400", w.Code)
			}
		})
	}
}
