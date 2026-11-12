// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
		{domain.ErrEmailInUse, http.StatusConflict},
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
	w := callError(&pgconn.PgError{Code: "23514", ConstraintName: "applications_attendance_check"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for check_violation, got %d", w.Code)
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
		w := callError(&pgconn.PgError{Code: tc.code})
		if w.Code != tc.want {
			t.Errorf("SQLSTATE %s: expected %d, got %d", tc.code, tc.want, w.Code)
		}
	}
}

// Server faults log at error level with the error; rejections a monitor should count log at warn
// level with only the sentinel or SQLSTATE, since the wrapped message can carry an LFID. The
// message, status and reason are what the #379 monitors search on, so each is pinned.
func TestError_LogsServerFaultsAndRejections(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	const secret = "lfid-must-not-be-logged"
	cases := []struct {
		err                            error
		wantLevel, wantMsg, wantReason string
		wantStatus                     int
	}{
		{fmt.Errorf("bootstrap user: %w", domain.ErrEmailInUse), "WARN", "request rejected", domain.ErrEmailInUse.Error(), http.StatusConflict},
		{fmt.Errorf("%w: lfid %q has no LF account", domain.ErrIneligible, secret), "WARN", "request rejected", domain.ErrIneligible.Error(), http.StatusUnprocessableEntity},
		{fmt.Errorf("%w: cannot transition", domain.ErrInvalidStateTransition), "WARN", "request rejected", domain.ErrInvalidStateTransition.Error(), http.StatusConflict},
		{fmt.Errorf("%w: locked", domain.ErrStateLocked), "WARN", "request rejected", domain.ErrStateLocked.Error(), http.StatusConflict},
		{domain.ErrConflict, "WARN", "request rejected", domain.ErrConflict.Error(), http.StatusConflict},
		{&pgconn.PgError{Code: "23505", Detail: secret}, "WARN", "request rejected", "sqlstate 23505", http.StatusConflict},
		{fmt.Errorf("lookup: %w", domain.ErrUpstreamUnavailable), "ERROR", "upstream unavailable", "", 0},
		{errors.New("boom"), "ERROR", "internal error", "", 0},
	}
	for _, tc := range cases {
		buf.Reset()
		callError(tc.err)
		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("%v: no log line: %v (%q)", tc.err, err, buf.String())
		}
		if entry["level"] != tc.wantLevel || entry["msg"] != tc.wantMsg {
			t.Errorf("%v: level, msg = %v, %v; want %s, %s", tc.err, entry["level"], entry["msg"], tc.wantLevel, tc.wantMsg)
		}
		if tc.wantLevel == "ERROR" {
			if entry["error"] != tc.err.Error() {
				t.Errorf("%v: error = %v; want the error", tc.err, entry["error"])
			}
			continue
		}
		if entry["reason"] != tc.wantReason || entry["status"] != float64(tc.wantStatus) {
			t.Errorf("%v: reason, status = %v, %v; want %q, %d", tc.err, entry["reason"], entry["status"], tc.wantReason, tc.wantStatus)
		}
		if _, ok := entry["error"]; ok || strings.Contains(buf.String(), secret) {
			t.Errorf("%v: log line leaks the wrapped message: %s", tc.err, buf.String())
		}
	}

	for _, quiet := range []error{domain.ErrInvalidInput, domain.ErrUnauthorized, domain.ErrForbidden, domain.ErrProgramNotFound} {
		buf.Reset()
		callError(quiet)
		if buf.Len() != 0 {
			t.Errorf("%v: logged %q; want nothing", quiet, buf.String())
		}
	}
}

// The public text of a bare conflict is fixed; the sentinel's own text is for the logs.
func TestError_ConflictBody(t *testing.T) {
	var body errorBody
	if err := json.NewDecoder(callError(domain.ErrConflict).Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "conflict" {
		t.Errorf("body = %q; want %q", body.Error, "conflict")
	}
}
