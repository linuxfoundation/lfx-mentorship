// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package handler provides HTTP handler helpers.
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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

// decodeGuardedBody is decodeBody for a route whose Heimdall rule checks the
// object named by guardKey in the body. Heimdall reads that exact key, while
// encoding/json matches keys case-insensitively and keeps the last one, so a
// body naming guardKey twice could authorize one object and act on another; it
// is refused with a 400.
func decodeGuardedBody(w http.ResponseWriter, r *http.Request, v any, guardKey string) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err == nil {
		err = rejectDuplicateKey(body, guardKey)
	}
	if err == nil {
		err = json.NewDecoder(bytes.NewReader(body)).Decode(v)
	}
	if err != nil {
		Error(w, fmt.Errorf("%w: %s", domain.ErrInvalidInput, err.Error()))
		return false
	}
	return true
}

// rejectDuplicateKey fails unless body is a JSON object naming key at most once,
// compared case-insensitively.
func rejectDuplicateKey(body []byte, key string) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	if tok, err := dec.Token(); err != nil {
		return err
	} else if tok != json.Delim('{') {
		return errors.New("body must be a JSON object")
	}
	seen := false
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if name, _ := tok.(string); strings.EqualFold(name, key) {
			if seen {
				return fmt.Errorf("%s must appear once", key)
			}
			seen = true
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
	}
	return nil
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
		errors.Is(err, domain.ErrProjectNotFound):
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
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrEmailInUse):
		return http.StatusConflict, err.Error()
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
