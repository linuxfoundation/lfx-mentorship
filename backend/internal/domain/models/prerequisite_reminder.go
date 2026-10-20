// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// The prerequisite reminder schedule, in UTC calendar days, as the legacy reminder job used. The
// repository applies the same values in SQL, so only applications due a reminder are read.
const (
	// PrerequisiteReminderFirstDay is how many days after the application the first reminder goes out.
	PrerequisiteReminderFirstDay = 3
	// PrerequisiteReminderInterval is how many days apart the reminders after the first one are.
	PrerequisiteReminderInterval = 7
	// PrerequisiteReminderDaysBeforeEnd and PrerequisiteReminderFinalDaysBeforeEnd are the days
	// before the term's application end date that also get a reminder.
	PrerequisiteReminderDaysBeforeEnd      = 3
	PrerequisiteReminderFinalDaysBeforeEnd = 1
)

// PrerequisiteReminderCandidate is a pending mentee application, on a term taking applications in a
// published program, that still has a prerequisite task to submit.
type PrerequisiteReminderCandidate struct {
	ApplicationID      string
	CreatedOn          time.Time
	ApplicationEndDate time.Time
}

// IsPrerequisiteReminderDue reports whether now falls on a reminder day for an application created
// at createdOn on a term whose applications close on applicationEnd. Reminder days are 3 days after
// the application and every 7 days after that, plus 3 days and 1 day before applications close.
// Days are compared by UTC calendar day, so a once-daily run reminds a mentee at most once a day.
func IsPrerequisiteReminderDue(createdOn, applicationEnd, now time.Time) bool {
	today := utcDay(now)
	sinceCreated := daysBetween(utcDay(createdOn), today)
	if sinceCreated >= PrerequisiteReminderFirstDay && (sinceCreated-PrerequisiteReminderFirstDay)%PrerequisiteReminderInterval == 0 {
		return true
	}
	untilEnd := daysBetween(today, utcDay(applicationEnd))
	return untilEnd == PrerequisiteReminderDaysBeforeEnd || untilEnd == PrerequisiteReminderFinalDaysBeforeEnd
}

// daysBetween returns the whole days from one UTC midnight to another.
func daysBetween(from, to time.Time) int {
	return int(to.Sub(from) / (24 * time.Hour))
}
