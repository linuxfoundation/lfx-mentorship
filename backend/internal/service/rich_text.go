// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"fmt"
	"unicode/utf8"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

// maxRichTextLength caps the stored HTML of a program description or profile introduction, in
// characters. It matches LFX One's raw rich-text limit (MENTORSHIP_RICH_TEXT_RAW_MAX: a
// 3,000-character text limit times 6 for markup), so anything its editor saves fits.
const maxRichTextLength = 18_000

// validateRichText rejects rich text longer than maxRichTextLength.
func validateRichText(field string, value *string) error {
	if value != nil && utf8.RuneCountInString(*value) > maxRichTextLength {
		return fmt.Errorf("%w: %s must be at most %d characters", domain.ErrInvalidInput, field, maxRichTextLength)
	}
	return nil
}
