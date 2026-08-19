// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package domain defines shared domain errors and repository contracts.
package domain

import "errors"

// Sentinel errors — mapped to HTTP status codes in handler/respond.go.
var (
	ErrUserNotFound          = errors.New("user not found")
	ErrUserProfileNotFound   = errors.New("user profile not found")
	ErrProgramNotFound       = errors.New("program not found")
	ErrProgramTermNotFound   = errors.New("program term not found")
	ErrProgramMemberNotFound = errors.New("program member not found")
	ErrApplicationNotFound   = errors.New("application not found")
	ErrEnrollmentNotFound    = errors.New("enrollment not found")
	ErrTaskNotFound          = errors.New("task not found")

	ErrInvalidInput        = errors.New("invalid input")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrConflict            = errors.New("resource conflict")
	ErrUpstreamUnavailable = errors.New("upstream service unavailable")
)
