// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

// maxSearchLength caps a search term, in characters. Lists match the term with ILIKE against
// every candidate row, and the public directories take it from anonymous callers, so its length
// bounds the cost of each request.
const maxSearchLength = 128

// maxNameLength caps a stored person's or program name, in characters. The directories search
// these names, and an ILIKE match costs more the longer the name it scans.
const maxNameLength = 255

// normalizeSearch trims search and rejects one longer than maxSearchLength.
func normalizeSearch(search string) (string, error) {
	search = strings.TrimSpace(search)
	if utf8.RuneCountInString(search) > maxSearchLength {
		return "", fmt.Errorf("%w: search must be at most %d characters", domain.ErrInvalidInput, maxSearchLength)
	}
	return search, nil
}

// validateName rejects a name longer than maxNameLength.
func validateName(field string, value *string) error {
	if value != nil && utf8.RuneCountInString(*value) > maxNameLength {
		return fmt.Errorf("%w: %s must be at most %d characters", domain.ErrInvalidInput, field, maxNameLength)
	}
	return nil
}

// boundedName returns value, or nil when it is longer than maxNameLength. A name the LF
// directory supplies is dropped rather than failing the write it fills.
func boundedName(value *string) *string {
	if validateName("", value) != nil {
		return nil
	}
	return value
}

// validateUserNames rejects a user name, given name, or family name longer than maxNameLength.
func validateUserNames(name, givenName, familyName *string) error {
	if err := validateName("name", name); err != nil {
		return err
	}
	if err := validateName("given_name", givenName); err != nil {
		return err
	}
	return validateName("family_name", familyName)
}

// validateProfileNames rejects a profile first or last name longer than maxNameLength; the
// directories fall back to them when the user has no name.
func validateProfileNames(firstName, lastName *string) error {
	if err := validateName("first_name", firstName); err != nil {
		return err
	}
	return validateName("last_name", lastName)
}
