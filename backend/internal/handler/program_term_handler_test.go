// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

type stubProgramTermSvc struct {
	getByProgramAndID func(context.Context, string, string) (*models.ProgramTerm, error)
}

func (s *stubProgramTermSvc) GetByID(ctx context.Context, id string) (*models.ProgramTerm, error) {
	return &models.ProgramTerm{ID: id}, nil
}

func (s *stubProgramTermSvc) GetByProgramAndID(ctx context.Context, programID, id string) (*models.ProgramTerm, error) {
	if s.getByProgramAndID != nil {
		return s.getByProgramAndID(ctx, programID, id)
	}
	return &models.ProgramTerm{ID: id, ProgramID: programID}, nil
}

func (s *stubProgramTermSvc) ListByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error) {
	return []*models.ProgramTerm{{ID: "term-1", ProgramID: programID}}, &models.PaginationMeta{Total: 1}, nil
}
func (s *stubProgramTermSvc) ListManagementByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTermManagementRow, *models.PaginationMeta, error) {
	return []*models.ProgramTermManagementRow{}, &models.PaginationMeta{}, nil
}

func (s *stubProgramTermSvc) Create(ctx context.Context, input models.ProgramTermCreateInput) (*models.ProgramTerm, error) {
	return &models.ProgramTerm{ID: input.ID, ProgramID: input.ProgramID}, nil
}

func (s *stubProgramTermSvc) Update(ctx context.Context, id string, input models.ProgramTermUpdateInput) (*models.ProgramTerm, error) {
	return &models.ProgramTerm{ID: id}, nil
}

func (s *stubProgramTermSvc) Delete(ctx context.Context, id string) error { return nil }

func (s *stubProgramTermSvc) Close(ctx context.Context, id string) (*models.ProgramTerm, int, error) {
	return &models.ProgramTerm{ID: id}, 0, nil
}

func (s *stubProgramTermSvc) Reopen(ctx context.Context, id string) (*models.ProgramTerm, error) {
	return &models.ProgramTerm{ID: id}, nil
}

func TestProgramTermHandler_GetByID_UsesProgramScope(t *testing.T) {
	var gotProgramID, gotTermID string
	h := handler.NewProgramTermHandler(&stubProgramTermSvc{
		getByProgramAndID: func(_ context.Context, programID, termID string) (*models.ProgramTerm, error) {
			gotProgramID = programID
			gotTermID = termID
			return &models.ProgramTerm{ID: termID, ProgramID: programID, Name: "Spring", CreatedOn: time.Now()}, nil
		},
	}, &stubProgramSvc{})

	r := httptest.NewRequest(http.MethodGet, "/v1/programs/prog-1/terms/term-1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("programID", "prog-1")
	rctx.URLParams.Add("termID", "term-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetByID(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if gotProgramID != "prog-1" || gotTermID != "term-1" {
		t.Fatalf("wrong scope: got program=%q term=%q", gotProgramID, gotTermID)
	}
}

func TestProgramTermHandler_GetByID_DeletedTermReturns404(t *testing.T) {
	h := handler.NewProgramTermHandler(&stubProgramTermSvc{
		getByProgramAndID: func(_ context.Context, programID, termID string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: termID, ProgramID: programID, Status: models.ProgramTermStatusDeleted}, nil
		},
	}, &stubProgramSvc{})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/prog-1/terms/term-1", nil)
	r = requestWithChiParam(r, "programID", "prog-1")
	r = requestWithChiParam(r, "termID", "term-1")
	w := httptest.NewRecorder()
	h.GetByID(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
}

func TestProgramTermHandler_PublicReads_HiddenProgramReturns404(t *testing.T) {
	lfid := "owner"
	programs := &stubProgramSvc{
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Slug: id, Status: models.ProgramStatusHidden, LFID: &lfid}, nil
		},
	}
	h := handler.NewProgramTermHandler(&stubProgramTermSvc{
		getByProgramAndID: func(context.Context, string, string) (*models.ProgramTerm, error) {
			t.Fatal("term must not be read for a hidden program")
			return nil, nil
		},
	}, programs)

	list := httptest.NewRequest(http.MethodGet, "/v1/programs/prog-1/terms", nil)
	list = requestWithChiParam(list, "id", "prog-1")
	get := httptest.NewRequest(http.MethodGet, "/v1/programs/prog-1/terms/term-1", nil)
	get = requestWithChiParam(get, "programID", "prog-1")
	get = requestWithChiParam(get, "termID", "term-1")

	for name, tc := range map[string]struct {
		serve http.HandlerFunc
		req   *http.Request
	}{
		"list": {h.ListByProgram, list},
		"get":  {h.GetByID, get},
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.serve(w, tc.req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("got %d; want 404", w.Code)
			}
		})
	}
}
