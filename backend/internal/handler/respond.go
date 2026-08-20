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
	switch {
	case errors.Is(err, domain.ErrUserNotFound),
		errors.Is(err, domain.ErrUserProfileNotFound),
		errors.Is(err, domain.ErrProgramNotFound),
		errors.Is(err, domain.ErrProgramTermNotFound),
		errors.Is(err, domain.ErrProgramMemberNotFound),
		errors.Is(err, domain.ErrApplicationNotFound),
		errors.Is(err, domain.ErrTaskNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrInvalidStateTransition):
		return http.StatusConflict, err.Error()
	case errors.Is(err, domain.ErrStateLocked):
		return http.StatusConflict, err.Error()
	case errors.Is(err, domain.ErrIneligible):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, domain.ErrUpstreamUnavailable):
		return http.StatusServiceUnavailable, "upstream unavailable"
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

// Error writes a JSON error response derived from a domain error.
func Error(w http.ResponseWriter, err error) {
	status, msg := mapError(err)
	if status == http.StatusInternalServerError {
		slog.Error("internal error", "error", err)
	}
	JSON(w, status, errorBody{Error: msg})
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
