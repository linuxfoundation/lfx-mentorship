// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package handler provides HTTP handler helpers.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

type errorBody struct {
	Error string `json:"error"`
}

// maxBodyBytes is the maximum request body size accepted by decodeBody.
const maxBodyBytes = 1 << 20 // 1 MB

// JSON writes a JSON-encoded body with the given status code.
func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("json encode error", "error", err)
	}
}

// decodeBody limits r.Body to 1 MB and JSON-decodes it into v.
// On failure it writes a 400 response and returns false.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		Error(w, fmt.Errorf("%w: %s", domain.ErrInvalidInput, err.Error()))
		return false
	}
	return true
}

// mapError translates domain sentinel errors to HTTP status codes.
func mapError(err error) (int, string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503": // foreign_key_violation
			return http.StatusUnprocessableEntity, "referenced resource does not exist"
		case "23505": // unique_violation
			return http.StatusConflict, "a resource with that value already exists"
		case "23514": // check_violation — a value outside an allowed set reached the DB
			return http.StatusUnprocessableEntity, "a field value is not allowed"
		}
	}
	switch {
	case errors.Is(err, domain.ErrUserNotFound),
		errors.Is(err, domain.ErrUserProfileNotFound),
		errors.Is(err, domain.ErrMenteeNotFound),
		errors.Is(err, domain.ErrMentorNotFound),
		errors.Is(err, domain.ErrProgramNotFound),
		errors.Is(err, domain.ErrProgramTermNotFound),
		errors.Is(err, domain.ErrProgramMemberNotFound),
		errors.Is(err, domain.ErrApplicationNotFound),
		errors.Is(err, domain.ErrTaskNotFound),
		errors.Is(err, domain.ErrFileNotFound),
		errors.Is(err, domain.ErrAccountNotFound),
		errors.Is(err, domain.ErrProjectNotFound),
		errors.Is(err, domain.ErrInitiativeNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, domain.ErrPayloadTooLarge):
		return http.StatusRequestEntityTooLarge, err.Error()
	case errors.Is(err, domain.ErrUnsupportedMedia):
		return http.StatusUnsupportedMediaType, err.Error()
	case errors.Is(err, domain.ErrRangeNotSatisfiable):
		return http.StatusRequestedRangeNotSatisfiable, "range not satisfiable"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	}
	for _, r := range rejections {
		if errors.Is(err, r.sentinel) {
			if r.message == "" {
				return r.status, err.Error()
			}
			return r.status, r.message
		}
	}
	if errors.Is(err, domain.ErrUpstreamUnavailable) {
		return http.StatusServiceUnavailable, "upstream unavailable"
	}
	return http.StatusInternalServerError, "internal server error"
}

// rejections are the sentinels a 409 or 422 maps from, in match order. An empty message answers
// the client with the wrapped error's own text.
var rejections = []struct {
	sentinel error
	status   int
	message  string
}{
	{domain.ErrConflict, http.StatusConflict, "conflict"},
	{domain.ErrEmailInUse, http.StatusConflict, ""},
	{domain.ErrInvalidStateTransition, http.StatusConflict, ""},
	{domain.ErrStateLocked, http.StatusConflict, ""},
	{domain.ErrIneligible, http.StatusUnprocessableEntity, ""},
}

// Error writes a JSON error response derived from a domain error.
func Error(w http.ResponseWriter, err error) {
	status, msg := mapError(err)
	switch status {
	case http.StatusInternalServerError:
		slog.Error("internal error", "error", err)
	case http.StatusServiceUnavailable:
		slog.Error("upstream unavailable", "error", err)
	case http.StatusConflict, http.StatusUnprocessableEntity:
		slog.Warn("request rejected", "status", status, "reason", rejectionReason(err))
	}
	JSON(w, status, errorBody{Error: msg})
}

// rejectionReason names a rejection by its sentinel or SQLSTATE only: the wrapped message can
// carry an LFID or an email address, which must not reach the logs.
func rejectionReason(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return "sqlstate " + pgErr.Code
	}
	for _, r := range rejections {
		if errors.Is(err, r.sentinel) {
			return r.sentinel.Error()
		}
	}
	return "unknown"
}

// newInvalidInput is a convenience wrapper for creating ErrInvalidInput errors.
func newInvalidInput(msg string) error {
	return fmt.Errorf("%w: %s", domain.ErrInvalidInput, msg)
}

// parsePaginationParams parses ?limit= and ?offset= from r.
func parsePaginationParams(w http.ResponseWriter, r *http.Request) (limit, offset int, ok bool) {
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			Error(w, fmt.Errorf("%w: limit must be an integer", domain.ErrInvalidInput))
			return 0, 0, false
		}
		limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			Error(w, fmt.Errorf("%w: offset must be an integer", domain.ErrInvalidInput))
			return 0, 0, false
		}
		offset = n
	}
	return limit, offset, true
}
