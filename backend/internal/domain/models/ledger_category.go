// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// MentorshipCategoryType defines supported ledger transaction categories used by mentorship sync.
type MentorshipCategoryType string

const (
	MentorshipCategory MentorshipCategoryType = "mentorship"
)

// IsValid reports whether the category value is one of the allowed enum members.
func (c MentorshipCategoryType) IsValid() bool {
	switch c {
	case MentorshipCategory:
		return true
	}
	return false
}
