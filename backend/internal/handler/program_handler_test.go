// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

type stubProgramSvc struct {
	listCatalog func(context.Context, models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error)
	getCatalog  func(context.Context, string) (*models.ProgramCatalogItem, error)
	getByID     func(context.Context, string) (*models.Program, error)
	getBySlug   func(context.Context, string) (*models.Program, error)
	listMentees func(context.Context, string) ([]*models.ProgramCatalogMentee, error)
}

func (s *stubProgramSvc) GetByID(ctx context.Context, id string) (*models.Program, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return &models.Program{ID: id, Status: models.ProgramStatusPublished}, nil
}
func (s *stubProgramSvc) GetBySlug(ctx context.Context, id string) (*models.Program, error) {
	if s.getBySlug != nil {
		return s.getBySlug(ctx, id)
	}
	return &models.Program{ID: id, Status: models.ProgramStatusPublished}, nil
}
func (s *stubProgramSvc) List(context.Context, models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error) {
	return []*models.Program{}, &models.PaginationMeta{}, nil
}
func (s *stubProgramSvc) ListCatalog(ctx context.Context, f models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error) {
	if s.listCatalog != nil {
		return s.listCatalog(ctx, f)
	}
	return []*models.ProgramCatalogItem{}, &models.PaginationMeta{}, nil
}
func (s *stubProgramSvc) GetCatalog(ctx context.Context, id string) (*models.ProgramCatalogItem, error) {
	if s.getCatalog != nil {
		return s.getCatalog(ctx, id)
	}
	return &models.ProgramCatalogItem{Program: models.Program{ID: id}}, nil
}
func (s *stubProgramSvc) ListCatalogMentees(ctx context.Context, id string) ([]*models.ProgramCatalogMentee, error) {
	if s.listMentees != nil {
		return s.listMentees(ctx, id)
	}
	return []*models.ProgramCatalogMentee{}, nil
}
func (s *stubProgramSvc) Create(context.Context, models.ProgramCreateInput) (*models.Program, error) {
	return &models.Program{}, nil
}
func (s *stubProgramSvc) Update(context.Context, string, models.ProgramUpdateInput) (*models.Program, error) {
	return &models.Program{}, nil
}
func (s *stubProgramSvc) Delete(context.Context, string) error { return nil }
func (s *stubProgramSvc) ListSkills(context.Context, string) ([]*models.ProgramSkill, error) {
	return []*models.ProgramSkill{}, nil
}
func (s *stubProgramSvc) AddSkill(context.Context, string, models.ProgramSkillCreateInput) (*models.ProgramSkill, error) {
	return &models.ProgramSkill{}, nil
}
func (s *stubProgramSvc) DeleteSkill(context.Context, string) error { return nil }
func (s *stubProgramSvc) GetFundingStats(context.Context, string) (*models.ProgramFundingStats, error) {
	return &models.ProgramFundingStats{}, nil
}

func TestProgramHandler_ListCatalog_OK(t *testing.T) {
	var captured models.ProgramFilter
	h := handler.NewProgramHandler(&stubProgramSvc{
		listCatalog: func(_ context.Context, f models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error) {
			captured = f
			return []*models.ProgramCatalogItem{{
				Program: models.Program{ID: "p1", Name: "K8s"},
				Skills:  []string{"Go"},
				Terms:   []models.ProgramCatalogTerm{},
				Mentors: []models.ProgramCatalogMentor{},
			}}, &models.PaginationMeta{Total: 1, Limit: 20, Offset: 0}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/catalog?search=kube&skill=Go&status=acceptance&sortBy=name_asc&limit=20&offset=0", nil)
	w := httptest.NewRecorder()
	h.ListCatalog(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if captured.Search != "kube" || captured.Skill != "Go" || captured.Limit != 20 || captured.DiscoveryStatus != "acceptance" || captured.SortBy != "name_asc" {
		t.Errorf("filter = %+v; want search=kube skill=Go status=acceptance sortBy=name_asc limit=20", captured)
	}
	var body struct {
		Data []models.ProgramCatalogItem `json:"data"`
		Meta models.PaginationMeta       `json:"meta"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "p1" || len(body.Data[0].Skills) != 1 {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestProgramHandler_ListCatalog_InvalidLimit(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/catalog?limit=abc", nil)
	w := httptest.NewRecorder()
	h.ListCatalog(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d; want 400", w.Code)
	}
}

func TestProgramHandler_GetCatalog_HiddenReturns404(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getCatalog: func(_ context.Context, id string) (*models.ProgramCatalogItem, error) {
			lfid := "owner"
			return &models.ProgramCatalogItem{Program: models.Program{
				ID:     id,
				Status: models.ProgramStatusHidden,
				LFID:   &lfid,
			}}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/catalog", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.GetCatalog(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_GetCatalog_DraftReturns404(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getCatalog: func(_ context.Context, id string) (*models.ProgramCatalogItem, error) {
			return &models.ProgramCatalogItem{Program: models.Program{
				ID:     id,
				Status: models.ProgramStatusDraft,
			}}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/catalog", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.GetCatalog(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_GetCatalog_RejectedReturns404(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getCatalog: func(_ context.Context, id string) (*models.ProgramCatalogItem, error) {
			return &models.ProgramCatalogItem{Program: models.Program{
				ID:     id,
				Status: models.ProgramStatusRejected,
			}}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/catalog", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.GetCatalog(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_GetCatalog_NotFound(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getCatalog: func(context.Context, string) (*models.ProgramCatalogItem, error) {
			return nil, domain.ErrProgramNotFound
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/missing/catalog", nil)
	r = requestWithChiParam(r, "id", "missing")
	w := httptest.NewRecorder()
	h.GetCatalog(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_ListCatalogMentees_OK(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusPublished}, nil
		},
		listMentees: func(_ context.Context, id string) ([]*models.ProgramCatalogMentee, error) {
			if id != "p1" {
				t.Errorf("program id = %q; want p1", id)
			}
			return []*models.ProgramCatalogMentee{{
				UserID:   "u1",
				Status:   "active",
				TermID:   "t1",
				TermName: "Spring 2026",
			}}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/mentees", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.ListCatalogMentees(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var body struct {
		Data []models.ProgramCatalogMentee `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].UserID != "u1" || body.Data[0].TermName != "Spring 2026" {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestProgramHandler_ListCatalogMentees_HiddenReturns404(t *testing.T) {
	lfid := "owner"
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusHidden, LFID: &lfid}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/mentees", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.ListCatalogMentees(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_ListCatalogMentees_NotFound(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(context.Context, string) (*models.Program, error) {
			return nil, domain.ErrProgramNotFound
		},
		getBySlug: func(context.Context, string) (*models.Program, error) {
			return nil, domain.ErrProgramNotFound
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/missing/mentees", nil)
	r = requestWithChiParam(r, "id", "missing")
	w := httptest.NewRecorder()
	h.ListCatalogMentees(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
}
