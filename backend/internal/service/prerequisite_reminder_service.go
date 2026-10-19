// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
)

var prerequisiteReminderSvcTracer = otel.Tracer("prerequisite-reminder-service")

// PrerequisiteReminderService reminds pending mentees of the prerequisite tasks they have not submitted.
type PrerequisiteReminderService struct {
	apps     domain.ApplicationRepository
	notifier domain.Notifier
}

// NewPrerequisiteReminderService returns a PrerequisiteReminderService.
func NewPrerequisiteReminderService(apps domain.ApplicationRepository, notifier domain.Notifier) *PrerequisiteReminderService {
	return &PrerequisiteReminderService{apps: apps, notifier: notifier}
}

// SendReminders queues a reminder for every application due one at now and returns how many it
// queued. The repository selects only due applications; each is checked against
// models.IsPrerequisiteReminderDue, the unit-tested rule, before it is sent. A listing failure is
// returned before anything is queued, so the run can be retried without emailing anyone twice. The
// notifier sends in the background and logs a failed send without stopping the others; the caller
// must wait for it before exiting.
func (s *PrerequisiteReminderService) SendReminders(ctx context.Context, now time.Time) (int, error) {
	ctx, span := prerequisiteReminderSvcTracer.Start(ctx, "PrerequisiteReminderService.SendReminders")
	defer span.End()

	due, err := s.apps.ListPrerequisiteRemindersDue(ctx, now)
	if err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("list prerequisite reminders due: %w", err)
	}
	reminded := 0
	for _, c := range due {
		if !models.IsPrerequisiteReminderDue(c.CreatedOn, c.ApplicationEndDate, now) {
			continue
		}
		s.notifier.NotifyMenteePrerequisiteReminder(ctx, c.ApplicationID)
		reminded++
	}
	return reminded, nil
}
