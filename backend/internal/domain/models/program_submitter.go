// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

// ProgramSubmitter is the user who submitted a program (`programs.lfid`), as its
// Program Admins and reviewers see it. Name, email and avatar are absent when
// the submitter has no Mentorship user row, or the row has no value for them.
type ProgramSubmitter struct {
	LFID      string  `json:"lfid"`
	Name      *string `json:"name,omitempty"`
	Email     *string `json:"email,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}
