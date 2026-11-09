// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import "strings"

// likeEscaper escapes the LIKE metacharacters, so a search term matches only as written.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// containsPattern returns the ILIKE pattern matching search as a literal substring.
func containsPattern(search string) string {
	return "%" + likeEscaper.Replace(search) + "%"
}
