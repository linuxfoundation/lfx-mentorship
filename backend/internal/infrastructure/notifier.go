// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package infrastructure

import (
	"context"
	"log/slog"
)

// LogNotifier is a stub Notifier that logs every notification via slog.
type LogNotifier struct {
	logger *slog.Logger
}

// NewLogNotifier returns a LogNotifier backed by logger.
func NewLogNotifier(logger *slog.Logger) *LogNotifier {
	return &LogNotifier{logger: logger}
}

// NotifyMentorInvited logs a mentor invitation event.
func (n *LogNotifier) NotifyMentorInvited(ctx context.Context, programID, userID, token string) {
	n.logger.InfoContext(ctx, "notify: mentor invited", "program_id", programID, "user_id", userID)
}

// NotifyMentorDeclined logs a mentor-declined event.
func (n *LogNotifier) NotifyMentorDeclined(ctx context.Context, programID, userID string) {
	n.logger.InfoContext(ctx, "notify: mentor declined", "program_id", programID, "user_id", userID)
}

// NotifyAdminTasksSubmitted logs a tasks-submitted event.
func (n *LogNotifier) NotifyAdminTasksSubmitted(ctx context.Context, applicationID string) {
	n.logger.InfoContext(ctx, "notify: all tasks submitted", "application_id", applicationID)
}

// NotifyMenteeAccepted logs a mentee-accepted event.
func (n *LogNotifier) NotifyMenteeAccepted(ctx context.Context, applicationID, attendanceType string) {
	n.logger.InfoContext(ctx, "notify: mentee accepted", "application_id", applicationID, "attendance_type", attendanceType)
}
