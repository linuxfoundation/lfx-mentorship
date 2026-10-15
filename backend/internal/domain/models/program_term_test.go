// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"
	"time"
)

func TestProgramTerm_AcceptsApplications(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	lastSecond := time.Date(2026, 8, 1, 23, 59, 59, 0, time.UTC)
	// A start stored mid-day still opens the window at 00:00 UTC that day.
	midDayStart := time.Date(2026, 6, 1, 15, 0, 0, 0, time.UTC)
	// 02:00 on 2 August in UTC+3 is still 1 August in UTC.
	eastOfUTC := time.Date(2026, 8, 2, 2, 0, 0, 0, time.FixedZone("UTC+3", 3*60*60))
	cases := map[string]struct {
		term ProgramTerm
		now  time.Time
		want bool
	}{
		"open inside window":            {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, start.Add(time.Hour), true},
		"open at window start":          {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, start, true},
		"open through end day":          {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, lastSecond, true},
		"open before start time on day": {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &midDayStart, ApplicationEndDate: &end}, start, true},
		"open on end day in UTC":        {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, eastOfUTC, true},
		"open before window":            {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, start.Add(-time.Second), false},
		"open after window":             {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end}, lastSecond.Add(time.Second), false},
		"open without start date":       {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationEndDate: &end}, start, false},
		"open without end date":         {ProgramTerm{Status: ProgramTermStatusOpen, ApplicationStartDate: &start}, start, false},
		"open without window":           {ProgramTerm{Status: ProgramTermStatusOpen}, start, false},
		"closed inside window":          {ProgramTerm{Status: ProgramTermStatusClosed, ApplicationStartDate: &start, ApplicationEndDate: &end}, start.Add(time.Hour), false},
		"deleted without window":        {ProgramTerm{Status: ProgramTermStatusDeleted}, start, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.term.AcceptsApplications(tc.now); got != tc.want {
				t.Fatalf("AcceptsApplications = %v; want %v", got, tc.want)
			}
		})
	}
}

// TestProgramTerm_DiscoveryLabelMatchesAcceptsApplications holds the public label to the apply
// rule: a term shows "Apply Now" exactly when it takes applications.
func TestProgramTerm_DiscoveryLabelMatchesAcceptsApplications(t *testing.T) {
	start := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	terms := map[string]ProgramTerm{
		"windowed":     {Status: ProgramTermStatusOpen, ApplicationStartDate: &start, ApplicationEndDate: &end},
		"no start":     {Status: ProgramTermStatusOpen, ApplicationEndDate: &end},
		"no end":       {Status: ProgramTermStatusOpen, ApplicationStartDate: &start},
		"closed":       {Status: ProgramTermStatusClosed, ApplicationStartDate: &start, ApplicationEndDate: &end},
		"no window":    {Status: ProgramTermStatusOpen},
		"deleted term": {Status: ProgramTermStatusDeleted},
	}
	moments := []time.Time{
		start.Add(-24 * time.Hour), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), start,
		time.Date(2026, 8, 1, 23, 59, 59, 0, time.UTC), time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
	}
	for name, term := range terms {
		for _, now := range moments {
			if apply, accepts := term.DiscoveryLabel(now) == "Apply Now", term.AcceptsApplications(now); apply != accepts {
				t.Fatalf("%s at %s: label %q but AcceptsApplications = %v", name, now, term.DiscoveryLabel(now), accepts)
			}
		}
	}
}
