// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"
	"time"
)

func TestProgramTerm_AcceptsApplications(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 1, 23, 59, 59, 0, time.UTC)
	cases := map[string]struct {
		term ProgramTerm
		now  time.Time
		want bool
	}{
		"open inside window":      {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, start.Add(time.Hour), true},
		"open at window start":    {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, start, true},
		"open at window end":      {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, end, true},
		"open before window":      {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, start.Add(-time.Second), false},
		"open after window":       {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, end.Add(time.Second), false},
		"open without start date": {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationEndDate: &end}, start.Add(-365 * 24 * time.Hour), true},
		"open without end date":   {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start}, end.Add(365 * 24 * time.Hour), true},
		"open without window":     {ProgramTerm{Status: ProgramTermStatusOpen}, start, true},
		"closed inside window":    {ProgramTerm{Status: ProgramTermStatusClosed, ApplicationStartDate: &start, ApplicationEndDate: &end}, start.Add(time.Hour), false},
		"deleted without window":  {ProgramTerm{Status: ProgramTermStatusDeleted}, start, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.term.AcceptsApplications(tc.now); got != tc.want {
				t.Fatalf("AcceptsApplications = %v; want %v", got, tc.want)
			}
		})
	}
}
