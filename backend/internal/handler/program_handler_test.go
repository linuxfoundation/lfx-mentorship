// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/auth"
)

type stubProgramSvc struct {
	listCatalog                func(context.Context, models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error)
	getCatalog                 func(context.Context, string) (*models.ProgramCatalogItem, error)
	getByID                    func(context.Context, string) (*models.Program, error)
	getBySlug                  func(context.Context, string) (*models.Program, error)
	getManagementSummary       func(context.Context, string) (*models.ProgramManagementSummary, error)
	nameAvailable              func(context.Context, string, string) (bool, error)
	listMentees                func(context.Context, string) ([]*models.ProgramCatalogMentee, error)
	listMine                   func(context.Context, string, models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error)
	listSkills                 func(context.Context, string) ([]*models.ProgramSkill, error)
	deleteSkill                func(context.Context, string, string, string) error
	getCategorizedTransactions func(context.Context, string, string, bool, int, int) (*models.ProgramCategorizedTransactions, error)
	getProgramSponsors         func(context.Context, string, string, bool, bool) ([]models.ProgramSponsor, error)
	createEnrollment           func(context.Context, models.ProgramEnrollmentInput) (*models.Program, error)
	update                     func(context.Context, string, models.ProgramUpdateInput) (*models.Program, error)
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
func (s *stubProgramSvc) GetEnrollmentTemplate(context.Context, string) (*models.ProgramEnrollmentTemplate, error) {
	return &models.ProgramEnrollmentTemplate{}, nil
}
func (s *stubProgramSvc) GetManagementSummary(ctx context.Context, id string) (*models.ProgramManagementSummary, error) {
	if s.getManagementSummary != nil {
		return s.getManagementSummary(ctx, id)
	}
	return &models.ProgramManagementSummary{}, nil
}
func (s *stubProgramSvc) GetHeaderProjection(ctx context.Context, id string) (*models.ProgramHeaderProjection, error) {
	return &models.ProgramHeaderProjection{Program: &models.Program{ID: id}}, nil
}
func (s *stubProgramSvc) NameAvailable(ctx context.Context, name, excludeProgramID string) (bool, error) {
	if s.nameAvailable != nil {
		return s.nameAvailable(ctx, name, excludeProgramID)
	}
	return true, nil
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
func (s *stubProgramSvc) ListMine(ctx context.Context, userID string, f models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error) {
	if s.listMine != nil {
		return s.listMine(ctx, userID, f)
	}
	return []*models.AdministeredProgram{}, &models.PaginationMeta{}, nil
}
func (s *stubProgramSvc) Create(context.Context, models.ProgramCreateInput) (*models.Program, error) {
	return &models.Program{}, nil
}
func (s *stubProgramSvc) CreateEnrollment(ctx context.Context, input models.ProgramEnrollmentInput) (*models.Program, error) {
	if s.createEnrollment != nil {
		return s.createEnrollment(ctx, input)
	}
	return s.Create(ctx, input.Program)
}
func (s *stubProgramSvc) Update(ctx context.Context, id string, input models.ProgramUpdateInput) (*models.Program, error) {
	if s.update != nil {
		return s.update(ctx, id, input)
	}
	return &models.Program{}, nil
}
func (s *stubProgramSvc) Decide(context.Context, string, models.ProgramStatus) (*models.Program, error) {
	return &models.Program{}, nil
}
func (s *stubProgramSvc) Delete(context.Context, string) error { return nil }
func (s *stubProgramSvc) ListSkills(ctx context.Context, programID string) ([]*models.ProgramSkill, error) {
	if s.listSkills != nil {
		return s.listSkills(ctx, programID)
	}
	return []*models.ProgramSkill{}, nil
}
func (s *stubProgramSvc) AddSkill(context.Context, string, models.ProgramSkillCreateInput) (*models.ProgramSkill, error) {
	return &models.ProgramSkill{}, nil
}
func (s *stubProgramSvc) DeleteSkill(ctx context.Context, programID, skillID, actorID string) error {
	if s.deleteSkill != nil {
		return s.deleteSkill(ctx, programID, skillID, actorID)
	}
	return nil
}
func (s *stubProgramSvc) GetFundingStats(context.Context, string) (*models.ProgramFundingStats, error) {
	return &models.ProgramFundingStats{}, nil
}
func (s *stubProgramSvc) GetCategorizedTransactions(ctx context.Context, programID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
	if s.getCategorizedTransactions != nil {
		return s.getCategorizedTransactions(ctx, programID, categoryType, subscriptionOnly, limit, offset)
	}
	return &models.ProgramCategorizedTransactions{}, nil
}
func (s *stubProgramSvc) GetProgramSponsors(ctx context.Context, programID, categoryType string, subscriptionOnly bool, aggregate bool) ([]models.ProgramSponsor, error) {
	if s.getProgramSponsors != nil {
		return s.getProgramSponsors(ctx, programID, categoryType, subscriptionOnly, aggregate)
	}
	return []models.ProgramSponsor{}, nil
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
				Status:   models.ApplicationStatusAccepted,
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
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Slug: id, Status: models.ProgramStatusHidden, LFID: &lfid}, nil
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

func TestProgramHandler_GetCategorizedTransactions_OK(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusPublished}, nil
		},
		getCategorizedTransactions: func(_ context.Context, programID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
			if programID != "p1" {
				t.Errorf("programID = %q; want p1", programID)
			}
			if categoryType != "mentorship" {
				t.Errorf("categoryType = %q; want mentorship", categoryType)
			}
			if !subscriptionOnly {
				t.Error("subscriptionOnly = false; want true")
			}
			if limit != 20 || offset != 5 {
				t.Errorf("limit/offset = %d/%d; want 20/5", limit, offset)
			}
			return &models.ProgramCategorizedTransactions{
				IndividualTransactions:   []models.ProgramTransaction{{ID: "ind-1", DonorType: "individual", AmountCents: 100}},
				OrganizationTransactions: []models.ProgramTransaction{{ID: "org-1", DonorType: "organization", AmountCents: 250}},
				TotalCount:               2,
				Limit:                    20,
				Offset:                   5,
			}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/transactions?categoryType=mentorship&subscriptionOnly=true&limit=20&offset=5", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.GetCategorizedTransactions(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}

	var body models.ProgramCategorizedTransactions
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.IndividualTransactions) != 1 || len(body.OrganizationTransactions) != 1 {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestProgramHandler_GetCategorizedTransactions_DefaultsAndSlugFallback(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return nil, domain.ErrProgramNotFound
		},
		getBySlug: func(_ context.Context, slug string) (*models.Program, error) {
			return &models.Program{ID: "resolved-program-id", Slug: slug, Status: models.ProgramStatusPublished}, nil
		},
		getCategorizedTransactions: func(_ context.Context, programID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
			if programID != "resolved-program-id" {
				t.Errorf("programID = %q; want resolved-program-id", programID)
			}
			if categoryType != "" {
				t.Errorf("categoryType = %q; want empty string for service default", categoryType)
			}
			if subscriptionOnly {
				t.Error("subscriptionOnly = true; want false")
			}
			if limit != 10 || offset != 0 {
				t.Errorf("limit/offset = %d/%d; want 10/0", limit, offset)
			}
			return &models.ProgramCategorizedTransactions{}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/kubernetes/transactions", nil)
	r = requestWithChiParam(r, "id", "kubernetes")
	w := httptest.NewRecorder()
	h.GetCategorizedTransactions(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
}

func TestProgramHandler_GetCategorizedTransactions_HiddenReturns404(t *testing.T) {
	lfid := "owner"
	h := handler.NewProgramHandler(&stubProgramSvc{
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Slug: id, Status: models.ProgramStatusHidden, LFID: &lfid}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/transactions", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.GetCategorizedTransactions(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_ResolveID_BySlugReturnsCanonicalID(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return nil, domain.ErrProgramNotFound
		},
		getBySlug: func(_ context.Context, slug string) (*models.Program, error) {
			return &models.Program{ID: "prog-uuid-1", Slug: slug, Status: models.ProgramStatusPublished}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/resolve/program-creation", nil)
	r = requestWithChiParam(r, "id", "program-creation")
	w := httptest.NewRecorder()
	h.ResolveID(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["id"] != "prog-uuid-1" {
		t.Fatalf("id = %q; want prog-uuid-1", body["id"])
	}
}

func TestProgramHandler_ResolveID_UUIDShapedSlug_FallsBackOnNotFound(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			if id != "00000000-0000-0000-0000-000000000000" {
				t.Fatalf("id = %q; want UUID-shaped slug", id)
			}
			return nil, domain.ErrProgramNotFound
		},
		getBySlug: func(_ context.Context, slug string) (*models.Program, error) {
			return &models.Program{ID: "resolved-id", Slug: slug, Status: models.ProgramStatusPublished}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/resolve/00000000-0000-0000-0000-000000000000", nil)
	r = requestWithChiParam(r, "id", "00000000-0000-0000-0000-000000000000")
	w := httptest.NewRecorder()
	h.ResolveID(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
}

func TestProgramHandler_ResolveID_UUIDInput_OperationalErrorPropagates(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(_ context.Context, _ string) (*models.Program, error) {
			return nil, errors.New("db unavailable")
		},
		getBySlug: func(_ context.Context, _ string) (*models.Program, error) {
			t.Fatal("unexpected slug fallback on operational error")
			return nil, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/resolve/00000000-0000-0000-0000-000000000000", nil)
	r = requestWithChiParam(r, "id", "00000000-0000-0000-0000-000000000000")
	w := httptest.NewRecorder()
	h.ResolveID(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d; want 500", w.Code)
	}
}

func TestProgramHandler_ResolveID_HiddenReturns404(t *testing.T) {
	lfid := "owner"
	h := handler.NewProgramHandler(&stubProgramSvc{
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Slug: id, Status: models.ProgramStatusHidden, LFID: &lfid}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/resolve/p1", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.ResolveID(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_ResolveID_PendingReturns404ToAnonymous(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getBySlug: func(_ context.Context, slug string) (*models.Program, error) {
			return &models.Program{ID: "pending-uuid", Slug: slug, Status: models.ProgramStatusPending}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/resolve/my-pending", nil)
	r = requestWithChiParam(r, "id", "my-pending")
	w := httptest.NewRecorder()
	h.ResolveID(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_ResolveID_PendingReturns404ToAuthenticatedNonOwner(t *testing.T) {
	owner := "owner"
	h := handler.NewProgramHandler(&stubProgramSvc{
		getBySlug: func(_ context.Context, slug string) (*models.Program, error) {
			return &models.Program{ID: "pending-uuid", Slug: slug, Status: models.ProgramStatusPending, LFID: &owner}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/resolve/my-pending", nil)
	r = requestWithChiParam(r, "id", "my-pending")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "someone", Username: "someone"}))
	w := httptest.NewRecorder()
	h.ResolveID(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_ResolveID_PendingResolvesForOwner(t *testing.T) {
	owner := "owner"
	h := handler.NewProgramHandler(&stubProgramSvc{
		getBySlug: func(_ context.Context, slug string) (*models.Program, error) {
			return &models.Program{ID: "pending-uuid", Slug: slug, Status: models.ProgramStatusPending, LFID: &owner}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/resolve/my-pending", nil)
	r = requestWithChiParam(r, "id", "my-pending")
	r = r.WithContext(auth.ContextWithPrincipal(r.Context(), &models.Principal{UserID: "owner-user", Username: owner}))
	w := httptest.NewRecorder()
	h.ResolveID(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["id"] != "pending-uuid" {
		t.Fatalf("id = %q; want pending-uuid", body["id"])
	}
}

func TestProgramHandler_ResolveID_SubmittedReturns404ToAnonymous(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Slug: id, Status: models.ProgramStatusSubmitted}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/resolve/p1", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.ResolveID(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_GetCatalog_PendingReturnsOK(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getCatalog: func(_ context.Context, id string) (*models.ProgramCatalogItem, error) {
			return &models.ProgramCatalogItem{
				Program: models.Program{ID: id, Name: "Pending Program", Status: models.ProgramStatusPending},
				Skills:  []string{},
				Terms:   []models.ProgramCatalogTerm{},
				Mentors: []models.ProgramCatalogMentor{},
			}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/pending-1/catalog", nil)
	r = requestWithChiParam(r, "id", "pending-1")
	w := httptest.NewRecorder()
	h.GetCatalog(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
}

func TestProgramHandler_GetProgramSponsors_OK(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusPublished}, nil
		},
		getProgramSponsors: func(_ context.Context, programID, categoryType string, subscriptionOnly bool, aggregate bool) ([]models.ProgramSponsor, error) {
			if programID != "p1" {
				t.Errorf("programID = %q; want p1", programID)
			}
			if categoryType != "mentorship" {
				t.Errorf("categoryType = %q; want mentorship", categoryType)
			}
			if !subscriptionOnly {
				t.Error("subscriptionOnly = false; want true")
			}
			if aggregate {
				t.Error("aggregate = true; want false")
			}
			return []models.ProgramSponsor{{ID: "gmc", Name: "GMC", AmountCents: 250000}}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/sponsors?categoryType=mentorship&subscriptionOnly=true", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.GetProgramSponsors(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}

	var body struct {
		Data []models.ProgramSponsor `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "gmc" {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestProgramHandler_GetProgramSponsors_AggregateQueryParam(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusPublished}, nil
		},
		getProgramSponsors: func(_ context.Context, _ string, _ string, _ bool, aggregate bool) ([]models.ProgramSponsor, error) {
			if !aggregate {
				t.Fatal("aggregate = false; want true")
			}
			return []models.ProgramSponsor{}, nil
		},
	})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/sponsors?aggregate=aggregate", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.GetProgramSponsors(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
}

func TestProgramHandler_GetProgramSponsors_HiddenReturns404(t *testing.T) {
	lfid := "owner"
	h := handler.NewProgramHandler(&stubProgramSvc{
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Slug: id, Status: models.ProgramStatusHidden, LFID: &lfid}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/sponsors", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.GetProgramSponsors(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
}

func TestProgramHandler_PublicSubResources_HiddenReturns404(t *testing.T) {
	lfid := "owner"
	svc := &stubProgramSvc{
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Slug: id, Status: models.ProgramStatusHidden, LFID: &lfid}, nil
		},
		listSkills: func(context.Context, string) ([]*models.ProgramSkill, error) {
			t.Fatal("skills must not be read for a hidden program")
			return nil, nil
		},
	}
	h := handler.NewProgramHandler(svc)
	for name, serve := range map[string]http.HandlerFunc{
		"skills":        h.ListSkills,
		"funding-stats": h.GetFundingStats,
		"header":        h.GetHeaderProjection,
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/"+name, nil)
			r = requestWithChiParam(r, "id", "p1")
			w := httptest.NewRecorder()
			serve(w, r)
			if w.Code != http.StatusNotFound {
				t.Fatalf("got %d; want 404", w.Code)
			}
		})
	}
}

func TestProgramHandler_DeleteSkill_RejectsSkillOutsideProgram(t *testing.T) {
	deleteCalled := false
	h := handler.NewProgramHandler(&stubProgramSvc{
		deleteSkill: func(_ context.Context, _, _, _ string) error {
			deleteCalled = true
			return domain.ErrProgramNotFound
		},
	})
	r := httptest.NewRequest(http.MethodDelete, "/v1/programs/p1/skills/skill-2", nil)
	r = requestWithPrincipal(r, "admin-1")
	r = requestWithChiParam(r, "id", "p1")
	r = requestWithChiParam(r, "skillId", "skill-2")
	w := httptest.NewRecorder()
	h.DeleteSkill(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
	if !deleteCalled {
		t.Fatal("expected delete to be called")
	}
}

func TestProgramHandler_DeleteSkill_DeletesWhenSkillBelongsToProgram(t *testing.T) {
	deleteCalled := false
	h := handler.NewProgramHandler(&stubProgramSvc{
		deleteSkill: func(_ context.Context, programID, skillID, actorID string) error {
			deleteCalled = true
			if programID != "p1" {
				t.Fatalf("programID = %q; want p1", programID)
			}
			if skillID != "skill-2" {
				t.Fatalf("skillID = %q; want skill-2", skillID)
			}
			if actorID != "admin-1" {
				t.Fatalf("actorID = %q; want admin-1", actorID)
			}
			return nil
		},
	})
	r := httptest.NewRequest(http.MethodDelete, "/v1/programs/p1/skills/skill-2", nil)
	r = requestWithPrincipal(r, "admin-1")
	r = requestWithChiParam(r, "id", "p1")
	r = requestWithChiParam(r, "skillId", "skill-2")
	w := httptest.NewRecorder()
	h.DeleteSkill(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("got %d; want 204", w.Code)
	}
	if !deleteCalled {
		t.Fatal("expected delete to be called")
	}
}

func TestProgramHandler_ListMine_ScopesToPrincipal(t *testing.T) {
	var gotUser string
	var gotFilter models.AdministeredProgramFilter
	h := handler.NewProgramHandler(&stubProgramSvc{
		listMine: func(_ context.Context, userID string, f models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error) {
			gotUser, gotFilter = userID, f
			return []*models.AdministeredProgram{{
				ID:          "p1",
				Name:        "GridFlow",
				AdminStatus: models.AdministeredProgramStatusOpen,
				Term:        &models.ProgramTerm{ID: "t1", ProgramID: "p1", Name: "Fall 2026", Status: models.ProgramTermStatusOpen},
				Stats:       models.ProgramHeaderStats{Mentors: 2, Mentees: 3, Graduated: 6},
			}}, &models.PaginationMeta{Total: 1, Limit: 5, Offset: 10}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/me/programs?search=grid&status=open&limit=5&offset=10", nil)
	r = requestWithPrincipal(r, "caller-user")
	w := httptest.NewRecorder()
	h.ListMine(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200: %s", w.Code, w.Body.String())
	}
	if gotUser != "caller-user" {
		t.Errorf("user = %q; want caller-user", gotUser)
	}
	want := models.AdministeredProgramFilter{Limit: 5, Offset: 10, Search: "grid", Status: models.AdministeredProgramStatusOpen}
	if gotFilter != want {
		t.Errorf("filter = %+v; want %+v", gotFilter, want)
	}
	var body struct {
		Data []models.AdministeredProgram `json:"data"`
		Meta models.PaginationMeta        `json:"meta"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "p1" || body.Data[0].Stats.Graduated != 6 || body.Meta.Total != 1 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if term := body.Data[0].Term; term == nil || term.ID != "t1" || term.Name != "Fall 2026" || term.Status != models.ProgramTermStatusOpen {
		t.Errorf("term = %+v; want open term t1 Fall 2026", term)
	}
}

func TestProgramHandler_ListMine_RequiresUserPrincipal(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		listMine: func(context.Context, string, models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error) {
			t.Fatal("service must not be called without a user principal")
			return nil, nil, nil
		},
	})
	for name, principal := range map[string]*models.Principal{
		"none": nil,
		"m2m":  {UserID: "client-id@clients", Username: "client-id@clients"},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/v1/me/programs", nil)
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

func TestProgramHandler_ListMine_InvalidStatus(t *testing.T) {
	h := handler.NewProgramHandler(&stubProgramSvc{
		listMine: func(context.Context, string, models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error) {
			return nil, nil, fmt.Errorf("%w: bad status", domain.ErrInvalidInput)
		},
	})
	r := requestWithPrincipal(httptest.NewRequest(http.MethodGet, "/v1/me/programs?status=bogus", nil), "caller-user")
	w := httptest.NewRecorder()
	h.ListMine(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d; want 400", w.Code)
	}
}

func TestProgramHandler_Update_PassesSkills(t *testing.T) {
	var captured models.ProgramUpdateInput
	h := handler.NewProgramHandler(&stubProgramSvc{
		update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
			captured = input
			return &models.Program{ID: "p1"}, nil
		},
	})
	r := httptest.NewRequest(http.MethodPatch, "/v1/programs/p1", strings.NewReader(`{"skills":["Go","Kubernetes"]}`))
	r = requestWithPrincipal(r, "admin-1")
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.Update(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200: %s", w.Code, w.Body.String())
	}
	if len(captured.Skills) != 2 || captured.Skills[0] != "Go" || captured.Skills[1] != "Kubernetes" {
		t.Fatalf("skills = %v; want [Go Kubernetes]", captured.Skills)
	}
}

func TestProgramHandler_Update_PassesTerms(t *testing.T) {
	var captured models.ProgramUpdateInput
	h := handler.NewProgramHandler(&stubProgramSvc{
		update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
			captured = input
			return &models.Program{ID: "p1"}, nil
		},
	})
	body := `{"terms":[{"id":"t1","name":"Fall 2026","start_date_time":"2026-11-01T00:00:00Z"},{"name":"Spring 2027"}]}`
	r := httptest.NewRequest(http.MethodPatch, "/v1/programs/p1", strings.NewReader(body))
	r = requestWithPrincipal(r, "admin-1")
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.Update(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200: %s", w.Code, w.Body.String())
	}
	if len(captured.Terms) != 2 || captured.Terms[0].ID != "t1" || captured.Terms[0].StartDateTime == nil || captured.Terms[1].ID != "" || captured.Terms[1].Name != "Spring 2027" {
		t.Fatalf("terms = %+v", captured.Terms)
	}
}

func TestProgramHandler_Update_PassesProject(t *testing.T) {
	var captured models.ProgramUpdateInput
	h := handler.NewProgramHandler(&stubProgramSvc{
		update: func(_ context.Context, _ string, input models.ProgramUpdateInput) (*models.Program, error) {
			captured = input
			return &models.Program{ID: "p1"}, nil
		},
	})
	body := `{"project_uid":"7cad5a8d-19d0-41a4-81a6-043453daf9ee","project_slug":"new-project","project_name":"New Project","project_logo_url":"https://example.com/logo.svg"}`
	r := httptest.NewRequest(http.MethodPatch, "/v1/programs/p1", strings.NewReader(body))
	r = requestWithPrincipal(r, "admin-1")
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.Update(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200: %s", w.Code, w.Body.String())
	}
	if captured.ProjectUID == nil || *captured.ProjectUID != "7cad5a8d-19d0-41a4-81a6-043453daf9ee" ||
		captured.ProjectSlug == nil || *captured.ProjectSlug != "new-project" ||
		captured.ProjectName == nil || *captured.ProjectName != "New Project" ||
		captured.ProjectLogoURL == nil || *captured.ProjectLogoURL != "https://example.com/logo.svg" {
		t.Fatalf("project fields = %v %v %v %v", captured.ProjectUID, captured.ProjectSlug, captured.ProjectName, captured.ProjectLogoURL)
	}
}

func TestProgramHandler_Create_MapsIndustry(t *testing.T) {
	var captured models.ProgramEnrollmentInput
	h := handler.NewProgramHandler(&stubProgramSvc{
		createEnrollment: func(_ context.Context, input models.ProgramEnrollmentInput) (*models.Program, error) {
			captured = input
			return &models.Program{ID: "p1"}, nil
		},
	})
	body := `{"projectId":"7cad5a8d-19d0-41a4-81a6-043453daf9ee","name":"CNCF Mentorship","industry":"Cloud Native","skills":["Go"],"termsAccepted":true,` +
		`"terms":[{"name":"Spring","startDate":"2026-03-01","endDate":"2026-05-31","applicationStartDate":"2026-01-15","applicationEndDate":"2026-02-15"}]}`
	r := requestWithPrincipal(httptest.NewRequest(http.MethodPost, "/v1/programs", strings.NewReader(body)), "creator-1")
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("got %d; want 201: %s", w.Code, w.Body.String())
	}
	if captured.Program.Industry == nil || *captured.Program.Industry != "Cloud Native" {
		t.Fatalf("industry = %v; want Cloud Native", captured.Program.Industry)
	}
}
