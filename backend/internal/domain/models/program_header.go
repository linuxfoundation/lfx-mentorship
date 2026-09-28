// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

type ProgramHeaderStats struct {
	Mentors   int `json:"mentors"`
	Mentees   int `json:"mentees"`
	Graduated int `json:"graduated"`
}

type ProgramHeaderProjection struct {
	Program    *Program           `json:"program"`
	ActiveTerm *ProgramTerm       `json:"active_term,omitempty"`
	Stats      ProgramHeaderStats `json:"stats"`
}
