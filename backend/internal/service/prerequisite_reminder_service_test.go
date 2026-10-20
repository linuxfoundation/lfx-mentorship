// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

func TestPrerequisiteReminderService_SendReminders(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	end := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	due := []*models.PrerequisiteReminderCandidate{
		{ApplicationID: "day-3", CreatedOn: time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC), ApplicationEndDate: end},
		{ApplicationID: "closing-tomorrow", CreatedOn: time.Date(2026, 3, 5, 9, 0, 0, 0, time.UTC), ApplicationEndDate: time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)},
		// The repository's SQL and models.IsPrerequisiteReminderDue disagreeing must not send.
		{ApplicationID: "day-2", CreatedOn: time.Date(2026, 3, 8, 9, 0, 0, 0, time.UTC), ApplicationEndDate: end},
	}
	var listedAt time.Time
	apps := &stubAppRepo{listRemindersDue: func(_ context.Context, at time.Time) ([]*models.PrerequisiteReminderCandidate, error) {
		listedAt = at
		return due, nil
	}}
	notifier := &stubNotifier{}

	reminded, err := service.NewPrerequisiteReminderService(apps, notifier).SendReminders(context.Background(), now)
	if err != nil {
		t.Fatalf("SendReminders: %v", err)
	}
	if !listedAt.Equal(now) {
		t.Errorf("listed reminders due at %s; want %s", listedAt, now)
	}
	if want := []string{"day-3", "closing-tomorrow"}; !slices.Equal(notifier.prerequisiteReminders, want) {
		t.Errorf("reminded %v; want %v", notifier.prerequisiteReminders, want)
	}
	if reminded != 2 {
		t.Errorf("reminded = %d; want 2", reminded)
	}
}

func TestPrerequisiteReminderService_SendRemindersListFailure(t *testing.T) {
	t.Parallel()
	listErr := errors.New("database down")
	apps := &stubAppRepo{listRemindersDue: func(context.Context, time.Time) ([]*models.PrerequisiteReminderCandidate, error) {
		return nil, listErr
	}}
	notifier := &stubNotifier{}

	_, err := service.NewPrerequisiteReminderService(apps, notifier).SendReminders(context.Background(), time.Now())
	if !errors.Is(err, listErr) {
		t.Fatalf("err = %v; want %v", err, listErr)
	}
	if len(notifier.prerequisiteReminders) != 0 {
		t.Errorf("reminded %v after a listing failure; want none", notifier.prerequisiteReminders)
	}
}
