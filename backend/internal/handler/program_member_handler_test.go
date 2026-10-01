// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

type stubProgramMemberSvc struct {
	getByID        func(context.Context, string) (*models.ProgramMember, error)
	listByProgram  func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error)
	listManagement func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMentorManagementRow, *models.PaginationMeta, error)
	update         func(context.Context, string, string, models.ProgramMemberUpdateInput, string) (*models.ProgramMember, error)
	listMine       func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMembership, *models.PaginationMeta, error)
	requestMentor  func(context.Context, string, string) (*models.ProgramMember, error)
	withdrawMine   func(context.Context, string, string) error
}

func (s *stubProgramMemberSvc) GetByID(ctx context.Context, id string) (*models.ProgramMember, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return &models.ProgramMember{}, nil
}
func (s *stubProgramMemberSvc) ListByProgram(ctx context.Context, programID string, f models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
	if s.listByProgram != nil {
		return s.listByProgram(ctx, programID, f)
	}
	return []*models.ProgramMember{}, &models.PaginationMeta{}, nil
}
func (s *stubProgramMemberSvc) ListMentorManagement(ctx context.Context, programID string, f models.ProgramMemberFilter) ([]*models.ProgramMentorManagementRow, *models.PaginationMeta, error) {
	if s.listManagement != nil {
		return s.listManagement(ctx, programID, f)
	}
	return []*models.ProgramMentorManagementRow{}, &models.PaginationMeta{}, nil
}
func (s *stubProgramMemberSvc) Create(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
	return &models.ProgramMember{}, nil
}
func (s *stubProgramMemberSvc) Update(ctx context.Context, programID, id string, in models.ProgramMemberUpdateInput, actorID string) (*models.ProgramMember, error) {
	if s.update != nil {
		return s.update(ctx, programID, id, in, actorID)
	}
	return &models.ProgramMember{}, nil
}
func (s *stubProgramMemberSvc) Delete(context.Context, string, string, string) error { return nil }
func (s *stubProgramMemberSvc) ListMine(ctx context.Context, userID string, f models.ProgramMemberFilter) ([]*models.ProgramMembership, *models.PaginationMeta, error) {
	if s.listMine != nil {
		return s.listMine(ctx, userID, f)
	}
	return []*models.ProgramMembership{}, &models.PaginationMeta{}, nil
}
func (s *stubProgramMemberSvc) RequestMentorship(ctx context.Context, programID, userID string) (*models.ProgramMember, error) {
	if s.requestMentor != nil {
		return s.requestMentor(ctx, programID, userID)
	}
	return &models.ProgramMember{}, nil
}
func (s *stubProgramMemberSvc) WithdrawMine(ctx context.Context, id, userID string) error {
	if s.withdrawMine != nil {
		return s.withdrawMine(ctx, id, userID)
	}
	return nil
}

// The public roster is active members only, and the caller must not be able to
// widen it by asking for another status.
func TestProgramMemberHandler_List_PinsActiveStatus(t *testing.T) {
	var captured models.ProgramMemberFilter
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		listByProgram: func(_ context.Context, _ string, f models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
			captured = f
			return []*models.ProgramMember{}, &models.PaginationMeta{}, nil
		},
	}, &stubProgramSvc{})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/members?status=pending&member_type=mentor", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if captured.Status != string(models.ProgramMemberStatusActive) {
		t.Errorf("Status = %q; want %q", captured.Status, models.ProgramMemberStatusActive)
	}
	if captured.MemberType != string(models.MemberTypeMentor) {
		t.Errorf("MemberType = %q; want %q", captured.MemberType, models.MemberTypeMentor)
	}
}

func TestProgramMemberHandler_List_HidesUnpublishedProgram(t *testing.T) {
	listCalled := false
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		listByProgram: func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
			listCalled = true
			return nil, nil, nil
		},
	}, &stubProgramSvc{
		getByID: func(context.Context, string) (*models.Program, error) {
			return &models.Program{ID: "p1", Status: models.ProgramStatusDraft}, nil
		},
		getBySlug: func(context.Context, string) (*models.Program, error) {
			return &models.Program{ID: "p1", Status: models.ProgramStatusDraft}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/members", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
	if listCalled {
		t.Fatal("listed members for unpublished program")
	}
}

// The members list is served unauthenticated, so it must never carry the
// member's email address, even though the service returns it.
func TestProgramMemberHandler_List_OmitsEmail(t *testing.T) {
	email := "admin@example.com"
	status := models.ProgramMemberStatusActive
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		listByProgram: func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
			return []*models.ProgramMember{{
				ID:         "m1",
				ProgramID:  "p1",
				UserID:     "u1",
				MemberType: models.MemberTypeProgramAdmin,
				Status:     &status,
				Email:      &email,
			}}, &models.PaginationMeta{Total: 1, Limit: 20, Offset: 0}, nil
		},
	}, &stubProgramSvc{})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/p1/members", nil)
	r = requestWithChiParam(r, "id", "p1")
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	raw := w.Body.String()
	if strings.Contains(raw, email) {
		t.Errorf("response leaks member email: %s", raw)
	}

	var body struct {
		Data []models.ProgramMember `json:"data"`
	}
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("got %d members; want 1", len(body.Data))
	}
	if body.Data[0].Email != nil {
		t.Errorf("Email = %q; want nil", *body.Data[0].Email)
	}
	// The rest of the row must survive redaction.
	if body.Data[0].ID != "m1" || body.Data[0].UserID != "u1" || body.Data[0].MemberType != models.MemberTypeProgramAdmin {
		t.Errorf("unexpected member: %+v", body.Data[0])
	}
}

func TestProgramMemberHandler_Update_RejectsMemberFromDifferentProgram(t *testing.T) {
	updateCalled := false
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		update: func(_ context.Context, _, _ string, _ models.ProgramMemberUpdateInput, _ string) (*models.ProgramMember, error) {
			updateCalled = true
			return nil, domain.ErrProgramMemberNotFound
		},
	}, &stubProgramSvc{})
	body := strings.NewReader(`{"status":"active"}`)
	r := httptest.NewRequest(http.MethodPatch, "/v1/programs/p1/members/m1", body)
	r = requestWithPrincipal(r, "admin-1")
	r = requestWithChiParam(r, "id", "p1")
	r = requestWithChiParam(r, "memberId", "m1")
	w := httptest.NewRecorder()
	h.Update(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
	if !updateCalled {
		t.Fatal("expected update to be called")
	}
}

func TestProgramMemberHandler_Delete_RejectsMemberFromDifferentProgram(t *testing.T) {
	updateCalled := false
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		update: func(_ context.Context, _, _ string, _ models.ProgramMemberUpdateInput, _ string) (*models.ProgramMember, error) {
			updateCalled = true
			return nil, domain.ErrProgramMemberNotFound
		},
	}, &stubProgramSvc{})
	r := httptest.NewRequest(http.MethodDelete, "/v1/programs/p1/members/m1", nil)
	r = requestWithPrincipal(r, "admin-1")
	r = requestWithChiParam(r, "id", "p1")
	r = requestWithChiParam(r, "memberId", "m1")
	w := httptest.NewRecorder()
	h.Delete(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d; want 404", w.Code)
	}
	if !updateCalled {
		t.Fatal("expected update to be called")
	}
}

// FR-009: a hidden program is a 404 for an anonymous caller here exactly as it
// is on the program itself, so holding a hidden program's ID does not expose
// its admins and mentors.
func TestProgramMemberHandler_List_HiddenReturns404(t *testing.T) {
	lfid := "owner"
	listed := false
	programID := "5fd9d697-2f84-4b6a-99d9-8e526ab58c58"
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		listByProgram: func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
			listed = true
			return []*models.ProgramMember{}, &models.PaginationMeta{}, nil
		},
	}, &stubProgramSvc{
		getByID: func(_ context.Context, id string) (*models.Program, error) {
			return &models.Program{ID: id, Status: models.ProgramStatusHidden, LFID: &lfid}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/"+programID+"/members", nil)
	r = requestWithChiParam(r, "id", programID)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("got %d; want 404", w.Code)
	}
	if listed {
		t.Error("roster was queried for a hidden program")
	}
}

// The slug form of the route must resolve to the program's ID before the
// roster is queried, not pass the slug through as though it were an ID.
func TestProgramMemberHandler_List_ResolvesSlugToID(t *testing.T) {
	var gotProgramID string
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		listByProgram: func(_ context.Context, programID string, _ models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
			gotProgramID = programID
			return []*models.ProgramMember{}, &models.PaginationMeta{}, nil
		},
	}, &stubProgramSvc{
		getByID: func(context.Context, string) (*models.Program, error) {
			return nil, domain.ErrProgramNotFound
		},
		getBySlug: func(_ context.Context, slug string) (*models.Program, error) {
			return &models.Program{ID: "p-uuid", Slug: slug, Status: models.ProgramStatusPublished}, nil
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/v1/programs/go-2026/members", nil)
	r = requestWithChiParam(r, "id", "go-2026")
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200", w.Code)
	}
	if gotProgramID != "p-uuid" {
		t.Errorf("programID = %q; want p-uuid", gotProgramID)
	}
}

// ── /me/program-memberships ──────────────────────────────────────────────────

func TestProgramMemberHandler_MeRoutes_NoPrincipal_Return401(t *testing.T) {
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{}, &stubProgramSvc{})
	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"list":     h.ListMine,
		"request":  h.RequestMine,
		"withdraw": h.WithdrawMine,
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/me/program-memberships", strings.NewReader(`{"program_id":"p1"}`))
			w := httptest.NewRecorder()
			call(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("got %d; want 401", w.Code)
			}
		})
	}
}

func TestProgramMemberHandler_ListMine_ScopesToPrincipal(t *testing.T) {
	var gotUser string
	var gotFilter models.ProgramMemberFilter
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		listMine: func(_ context.Context, userID string, f models.ProgramMemberFilter) ([]*models.ProgramMembership, *models.PaginationMeta, error) {
			gotUser, gotFilter = userID, f
			return []*models.ProgramMembership{{ID: "m1", ProgramName: "Program One"}}, &models.PaginationMeta{Total: 1}, nil
		},
	}, &stubProgramSvc{})
	r := httptest.NewRequest(http.MethodGet, "/v1/me/program-memberships?member_type=mentor&limit=5&offset=10", nil)
	r = requestWithPrincipal(r, "caller-user")
	w := httptest.NewRecorder()
	h.ListMine(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d; want 200: %s", w.Code, w.Body.String())
	}
	if gotUser != "caller-user" {
		t.Errorf("userID = %q; want caller-user", gotUser)
	}
	if gotFilter.MemberType != string(models.MemberTypeMentor) || gotFilter.Limit != 5 || gotFilter.Offset != 10 {
		t.Errorf("filter = %+v; want member_type=mentor limit=5 offset=10", gotFilter)
	}
	if !strings.Contains(w.Body.String(), `"program_name":"Program One"`) {
		t.Errorf("body missing program_name: %s", w.Body.String())
	}
}

func TestProgramMemberHandler_ListMine_InvalidMemberType_Returns400(t *testing.T) {
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		listMine: func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMembership, *models.PaginationMeta, error) {
			return nil, nil, domain.ErrInvalidInput
		},
	}, &stubProgramSvc{})
	r := httptest.NewRequest(http.MethodGet, "/v1/me/program-memberships?member_type=bogus", nil)
	r = requestWithPrincipal(r, "caller-user")
	w := httptest.NewRecorder()
	h.ListMine(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d; want 400", w.Code)
	}
}

// The user comes from the principal; a user_id in the body is ignored.
func TestProgramMemberHandler_RequestMine_UsesPrincipalNotBody(t *testing.T) {
	var gotProgram, gotUser string
	requested := models.ProgramMemberStatusRequested
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		requestMentor: func(_ context.Context, programID, userID string) (*models.ProgramMember, error) {
			gotProgram, gotUser = programID, userID
			return &models.ProgramMember{ID: "m1", ProgramID: programID, UserID: userID, MemberType: models.MemberTypeMentor, Status: &requested}, nil
		},
	}, &stubProgramSvc{})
	r := httptest.NewRequest(http.MethodPost, "/v1/me/program-memberships", strings.NewReader(`{"program_id":"p1","user_id":"someone-else"}`))
	r = requestWithPrincipal(r, "caller-user")
	w := httptest.NewRecorder()
	h.RequestMine(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("got %d; want 201: %s", w.Code, w.Body.String())
	}
	if gotProgram != "p1" || gotUser != "caller-user" {
		t.Errorf("RequestMentorship(%q, %q); want (p1, caller-user)", gotProgram, gotUser)
	}
	var body models.ProgramMember
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status == nil || *body.Status != models.ProgramMemberStatusRequested {
		t.Errorf("status = %v; want requested", body.Status)
	}
}

func TestProgramMemberHandler_RequestMine_MapsErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"missing program":  {domain.ErrProgramNotFound, http.StatusNotFound},
		"not published":    {domain.ErrInvalidInput, http.StatusBadRequest},
		"existing request": {domain.ErrConflict, http.StatusConflict},
	} {
		t.Run(name, func(t *testing.T) {
			h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
				requestMentor: func(context.Context, string, string) (*models.ProgramMember, error) { return nil, tc.err },
			}, &stubProgramSvc{})
			r := httptest.NewRequest(http.MethodPost, "/v1/me/program-memberships", strings.NewReader(`{"program_id":"p1"}`))
			r = requestWithPrincipal(r, "caller-user")
			w := httptest.NewRecorder()
			h.RequestMine(w, r)
			if w.Code != tc.want {
				t.Errorf("got %d; want %d", w.Code, tc.want)
			}
		})
	}
}

func TestProgramMemberHandler_RequestMine_MalformedBody_Returns400(t *testing.T) {
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		requestMentor: func(context.Context, string, string) (*models.ProgramMember, error) {
			t.Fatal("service must not be called for a malformed body")
			return nil, nil
		},
	}, &stubProgramSvc{})
	r := httptest.NewRequest(http.MethodPost, "/v1/me/program-memberships", strings.NewReader(`{`))
	r = requestWithPrincipal(r, "caller-user")
	w := httptest.NewRecorder()
	h.RequestMine(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d; want 400", w.Code)
	}
}

func TestProgramMemberHandler_WithdrawMine(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"withdrawn":          {nil, http.StatusNoContent},
		"not the caller row": {domain.ErrProgramMemberNotFound, http.StatusNotFound},
		"not withdrawable":   {domain.ErrInvalidStateTransition, http.StatusConflict},
	} {
		t.Run(name, func(t *testing.T) {
			var gotID, gotUser string
			h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
				withdrawMine: func(_ context.Context, id, userID string) error {
					gotID, gotUser = id, userID
					return tc.err
				},
			}, &stubProgramSvc{})
			r := httptest.NewRequest(http.MethodPost, "/v1/me/program-memberships/m1/withdraw", nil)
			r = requestWithPrincipal(r, "caller-user")
			r = requestWithChiParam(r, "id", "m1")
			w := httptest.NewRecorder()
			h.WithdrawMine(w, r)
			if w.Code != tc.want {
				t.Errorf("got %d; want %d", w.Code, tc.want)
			}
			if gotID != "m1" || gotUser != "caller-user" {
				t.Errorf("WithdrawMine(%q, %q); want (m1, caller-user)", gotID, gotUser)
			}
		})
	}
}
