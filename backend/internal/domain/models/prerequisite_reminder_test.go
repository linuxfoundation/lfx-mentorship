// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"
	"time"
)

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestIsPrerequisiteReminderDue(t *testing.T) {
	t.Parallel()
	// 2026-02-20 is day 50 after 2026-01-01, so no end-date case below also falls on a created-on day.
	created := utc("2026-01-01T23:59:00Z")
	end := utc("2026-02-20T23:00:00Z")
	cases := map[string]struct {
		created time.Time
		now     time.Time
		want    bool
	}{
		"application day":                {created, utc("2026-01-01T23:59:30Z"), false},
		"day 2 at its last second":       {created, utc("2026-01-03T23:59:59Z"), false},
		"day 3 at midnight":              {created, utc("2026-01-04T00:00:00Z"), true},
		"day 4":                          {created, utc("2026-01-05T14:30:00Z"), false},
		"day 9":                          {created, utc("2026-01-10T14:30:00Z"), false},
		"day 10":                         {created, utc("2026-01-11T14:30:00Z"), true},
		"day 17":                         {created, utc("2026-01-18T14:30:00Z"), true},
		"created-on compared in UTC":     {utc("2026-01-01T20:00:00-05:00"), utc("2026-01-04T14:30:00Z"), false},
		"created-on UTC day 3":           {utc("2026-01-01T20:00:00-05:00"), utc("2026-01-05T14:30:00Z"), true},
		"4 days before applications end": {created, utc("2026-02-16T14:30:00Z"), false},
		"3 days before applications end": {created, utc("2026-02-17T00:00:00Z"), true},
		"2 days before applications end": {created, utc("2026-02-18T14:30:00Z"), false},
		"1 day before applications end":  {created, utc("2026-02-19T23:59:59Z"), true},
		"applications end day":           {created, utc("2026-02-20T14:30:00Z"), false},
		"after applications end":         {created, utc("2026-02-21T14:30:00Z"), false},
		"end date compared in UTC":       {created, utc("2026-02-19T20:00:00-05:00"), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := IsPrerequisiteReminderDue(tc.created, end, tc.now); got != tc.want {
				t.Errorf("IsPrerequisiteReminderDue(%s, %s, %s) = %v; want %v", tc.created, end, tc.now, got, tc.want)
			}
		})
	}
}
