// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

// errorBody mirrors the JSON shape written by handler.Error.
type errorBody struct {
	Error string `json:"error"`
}

func callError(err error) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	handler.Error(w, err)
	return w
}

func TestError_NotFoundErrors(t *testing.T) {
	notFound := []error{
		domain.ErrUserNotFound,
		domain.ErrUserProfileNotFound,
		domain.ErrMenteeNotFound,
		domain.ErrMentorNotFound,
		domain.ErrProgramNotFound,
		domain.ErrProgramTermNotFound,
		domain.ErrProgramMemberNotFound,
		domain.ErrApplicationNotFound,
		domain.ErrTaskNotFound,
	}
	for _, err := range notFound {
		w := callError(err)
		if w.Code != http.StatusNotFound {
			t.Errorf("%v → %d; want 404", err, w.Code)
		}
	}
}

func TestError_WrappedSentinels(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{errors.New("oops"), http.StatusInternalServerError},
		{domain.ErrUnauthorized, http.StatusUnauthorized},
		{domain.ErrForbidden, http.StatusForbidden},
		{domain.ErrConflict, http.StatusConflict},
		{domain.ErrInvalidStateTransition, http.StatusConflict},
		{domain.ErrStateLocked, http.StatusConflict},
		{domain.ErrIneligible, http.StatusUnprocessableEntity},
		{domain.ErrUpstreamUnavailable, http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		w := callError(tc.err)
		if w.Code != tc.want {
			t.Errorf("%v → %d; want %d", tc.err, w.Code, tc.want)
		}
	}
}

func TestError_InvalidInput_Returns400(t *testing.T) {
	err := errors.Join(domain.ErrInvalidInput, errors.New("name is required"))
	w := callError(err)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d; want 400", w.Code)
	}
}

func TestError_WritesJSONBody(t *testing.T) {
	w := callError(domain.ErrProgramNotFound)
	var body errorBody
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error == "" {
		t.Error("expected non-empty error message in JSON body")
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
}

// A CHECK constraint violation is bad client input, not a server fault, so it
// must not surface as a 500.
func TestError_CheckViolation_IsNotInternalError(t *testing.T) {
	rec := httptest.NewRecorder()
	handler.Error(rec, &pgconn.PgError{Code: "23514", ConstraintName: "applications_attendance_check"})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for check_violation, got %d", rec.Code)
	}
}

func TestError_PgErrorMappings(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{"23503", http.StatusUnprocessableEntity}, // foreign_key_violation
		{"23505", http.StatusConflict},            // unique_violation
		{"23514", http.StatusUnprocessableEntity}, // check_violation
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		handler.Error(rec, &pgconn.PgError{Code: tc.code})
		if rec.Code != tc.want {
			t.Errorf("SQLSTATE %s: expected %d, got %d", tc.code, tc.want, rec.Code)
		}
	}
}
