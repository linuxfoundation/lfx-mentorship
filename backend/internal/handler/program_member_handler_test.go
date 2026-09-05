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

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

type stubProgramMemberSvc struct {
	listByProgram func(context.Context, string, models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error)
}

func (s *stubProgramMemberSvc) GetByID(context.Context, string) (*models.ProgramMember, error) {
	return &models.ProgramMember{}, nil
}
func (s *stubProgramMemberSvc) ListByProgram(ctx context.Context, programID string, f models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
	if s.listByProgram != nil {
		return s.listByProgram(ctx, programID, f)
	}
	return []*models.ProgramMember{}, &models.PaginationMeta{}, nil
}
func (s *stubProgramMemberSvc) Create(context.Context, string, models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
	return &models.ProgramMember{}, nil
}
func (s *stubProgramMemberSvc) Update(context.Context, string, models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
	return &models.ProgramMember{}, nil
}
func (s *stubProgramMemberSvc) Delete(context.Context, string) error { return nil }

// The public roster is active members only, and the caller must not be able to
// widen it by asking for another status.
func TestProgramMemberHandler_List_PinsActiveStatus(t *testing.T) {
	var captured models.ProgramMemberFilter
	h := handler.NewProgramMemberHandler(&stubProgramMemberSvc{
		listByProgram: func(_ context.Context, _ string, f models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
			captured = f
			return []*models.ProgramMember{}, &models.PaginationMeta{}, nil
		},
	})
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
	// member_type stays caller-controlled: every type on the roster is public.
	if captured.MemberType != "mentor" {
		t.Errorf("MemberType = %q; want mentor", captured.MemberType)
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
	})
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
