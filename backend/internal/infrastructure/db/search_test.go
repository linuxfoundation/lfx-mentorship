// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import "testing"

func TestContainsPattern_EscapesLikeMetacharacters(t *testing.T) {
	for search, want := range map[string]string{
		"ada":        `%ada%`,
		"100%":       `%100\%%`,
		"snake_case": `%snake\_case%`,
		`C:\dir`:     `%C:\\dir%`,
	} {
		if got := containsPattern(search); got != want {
			t.Errorf("containsPattern(%q) = %q; want %q", search, got, want)
		}
	}
}
