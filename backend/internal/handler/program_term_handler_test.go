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

	"github.com/go-chi/chi/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
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

func applyTargetRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/prog-1/terms/term-1/apply-target", nil)
	r = requestWithChiParam(r, "programID", "prog-1")
	return requestWithChiParam(r, "termID", "term-1")
}

// openTermInWindow returns an open term whose application window spans today.
func openTermInWindow(programID, termID string) *models.ProgramTerm {
	start, end := time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1)
	return &models.ProgramTerm{ID: termID, ProgramID: programID, Name: "Spring 2026", Status: models.ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}
}

func TestProgramTermHandler_GetApplyTarget_ReturnsNamesAndAcceptance(t *testing.T) {
	project := "Kubernetes"
	programs := &stubProgramSvc{
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Name: "LFX Mentorship", ProjectName: &project, Status: models.ProgramStatusPublished}, nil
		},
	}
	h := handler.NewProgramTermHandler(&stubProgramTermSvc{
		getByProgramAndID: func(_ context.Context, programID, termID string) (*models.ProgramTerm, error) {
			if programID != "prog-1" || termID != "term-1" {
				t.Errorf("wrong scope: program=%q term=%q", programID, termID)
			}
			return openTermInWindow(programID, termID), nil
		},
	}, programs)

	w := httptest.NewRecorder()
	h.GetApplyTarget(w, applyTargetRequest())

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var got models.ProgramTermApplyTarget
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := models.ProgramTermApplyTarget{ProgramName: "LFX Mentorship", ProjectName: "Kubernetes", TermName: "Spring 2026", AcceptingApplications: true}
	if got != want {
		t.Fatalf("got %+v; want %+v", got, want)
	}
}

func TestProgramTermHandler_GetApplyTarget_NoProjectOmitsProjectName(t *testing.T) {
	h := handler.NewProgramTermHandler(&stubProgramTermSvc{
		getByProgramAndID: func(_ context.Context, programID, termID string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: termID, ProgramID: programID, Name: "Spring 2026", Status: models.ProgramTermStatusClosed}, nil
		},
	}, &stubProgramSvc{})

	w := httptest.NewRecorder()
	h.GetApplyTarget(w, applyTargetRequest())

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, present := raw["project_name"]; present {
		t.Fatalf("project_name present without a project: %v", raw)
	}
	if raw["accepting_applications"] != false {
		t.Fatalf("accepting_applications = %v; want false for a closed term", raw["accepting_applications"])
	}
}

// A pending program stays visible to its admin, but submit refuses an unpublished program, so the
// apply page must not report it as accepting.
func TestProgramTermHandler_GetApplyTarget_UnpublishedProgramNotAccepting(t *testing.T) {
	pending := &stubProgramSvc{
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Name: "LFX Mentorship", Status: models.ProgramStatusPending}, nil
		},
	}
	h := handler.NewProgramTermHandler(&stubProgramTermSvc{
		getByProgramAndID: func(_ context.Context, programID, termID string) (*models.ProgramTerm, error) {
			return openTermInWindow(programID, termID), nil
		},
	}, pending)

	w := httptest.NewRecorder()
	h.GetApplyTarget(w, applyTargetRequest())

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	var got models.ProgramTermApplyTarget
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AcceptingApplications {
		t.Fatal("accepting_applications = true; want false for an unpublished program")
	}
}

func TestProgramTermHandler_GetApplyTarget_NotFound(t *testing.T) {
	lfid := "owner"
	hidden := &stubProgramSvc{
		getBySlug: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Slug: id, Status: models.ProgramStatusHidden, LFID: &lfid}, nil
		},
	}
	for name, tc := range map[string]struct {
		programs *stubProgramSvc
		term     func(context.Context, string, string) (*models.ProgramTerm, error)
	}{
		"hidden program": {hidden, func(context.Context, string, string) (*models.ProgramTerm, error) {
			t.Fatal("term must not be read for a hidden program")
			return nil, nil
		}},
		"deleted term": {&stubProgramSvc{}, func(_ context.Context, programID, termID string) (*models.ProgramTerm, error) {
			return &models.ProgramTerm{ID: termID, ProgramID: programID, Status: models.ProgramTermStatusDeleted}, nil
		}},
		"term not in program": {&stubProgramSvc{}, func(context.Context, string, string) (*models.ProgramTerm, error) {
			return nil, domain.ErrProgramTermNotFound
		}},
	} {
		t.Run(name, func(t *testing.T) {
			h := handler.NewProgramTermHandler(&stubProgramTermSvc{getByProgramAndID: tc.term}, tc.programs)
			w := httptest.NewRecorder()
			h.GetApplyTarget(w, applyTargetRequest())
			if w.Code != http.StatusNotFound {
				t.Fatalf("got %d; want 404", w.Code)
			}
		})
	}
}
