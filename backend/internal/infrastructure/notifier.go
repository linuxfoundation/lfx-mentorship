// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package infrastructure

import (
	"context"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// LogNotifier is a stub Notifier that logs every notification via slog.
type LogNotifier struct {
	logger *slog.Logger
}

// NewLogNotifier returns a LogNotifier backed by logger.
func NewLogNotifier(logger *slog.Logger) *LogNotifier {
	return &LogNotifier{logger: logger}
}

// NotifyProgramSubmitted logs a program-submitted event.
func (n *LogNotifier) NotifyProgramSubmitted(ctx context.Context, programID, submitterID string) {
	n.logger.InfoContext(ctx, "notify: program submitted", "program_id", programID, "submitter_id", submitterID)
}

// NotifyProgramDecided logs a program-decided event.
func (n *LogNotifier) NotifyProgramDecided(ctx context.Context, programID string, status models.ProgramStatus) {
	n.logger.InfoContext(ctx, "notify: program decided", "program_id", programID, "status", status)
}

// NotifyMentorInvited logs a mentor invitation event.
func (n *LogNotifier) NotifyMentorInvited(ctx context.Context, programID, userID, token string) {
	n.logger.InfoContext(ctx, "notify: mentor invited", "program_id", programID, "user_id", userID)
}

// NotifyMentorDeclined logs a mentor-declined event.
func (n *LogNotifier) NotifyMentorDeclined(ctx context.Context, programID, userID string) {
	n.logger.InfoContext(ctx, "notify: mentor declined", "program_id", programID, "user_id", userID)
}

// NotifyAdminMentorDeclined logs a mentor-declined-invite event.
func (n *LogNotifier) NotifyAdminMentorDeclined(ctx context.Context, programID, userID string) {
	n.logger.InfoContext(ctx, "notify: mentor declined invite", "program_id", programID, "user_id", userID)
}

// NotifyAdminMentorAccepted logs a mentor-accepted-invite event.
func (n *LogNotifier) NotifyAdminMentorAccepted(ctx context.Context, programID, userID string) {
	n.logger.InfoContext(ctx, "notify: mentor accepted invite", "program_id", programID, "user_id", userID)
}

// NotifyAdminMentorRequested logs a mentor-requested event.
func (n *LogNotifier) NotifyAdminMentorRequested(ctx context.Context, programID, userID string) {
	n.logger.InfoContext(ctx, "notify: mentor requested", "program_id", programID, "user_id", userID)
}

// NotifyAdminMentorWithdrew logs a mentor-withdrew-request event.
func (n *LogNotifier) NotifyAdminMentorWithdrew(ctx context.Context, programID, userID string) {
	n.logger.InfoContext(ctx, "notify: mentor withdrew request", "program_id", programID, "user_id", userID)
}

// NotifyAdminTasksSubmitted logs a tasks-submitted event.
func (n *LogNotifier) NotifyAdminTasksSubmitted(ctx context.Context, applicationID string) {
	n.logger.InfoContext(ctx, "notify: all tasks submitted", "application_id", applicationID)
}

// NotifyMenteeApplicationReceived logs a mentee-application-received event.
func (n *LogNotifier) NotifyMenteeApplicationReceived(ctx context.Context, applicationID string) {
	n.logger.InfoContext(ctx, "notify: mentee application received", "application_id", applicationID)
}

// NotifyMenteeAccepted logs a mentee-accepted event.
func (n *LogNotifier) NotifyMenteeAccepted(ctx context.Context, applicationID, attendanceType string) {
	n.logger.InfoContext(ctx, "notify: mentee accepted", "application_id", applicationID, "attendance_type", attendanceType)
}

// NotifyMenteeDeclined logs a mentee-declined event.
func (n *LogNotifier) NotifyMenteeDeclined(ctx context.Context, applicationID string) {
	n.logger.InfoContext(ctx, "notify: mentee declined", "application_id", applicationID)
}

// NotifyMenteeWithdrawn logs a mentee-withdrawn event.
func (n *LogNotifier) NotifyMenteeWithdrawn(ctx context.Context, applicationID string) {
	n.logger.InfoContext(ctx, "notify: mentee withdrawn", "application_id", applicationID)
}

// NotifyMenteeGraduated logs a mentee-graduated event.
func (n *LogNotifier) NotifyMenteeGraduated(ctx context.Context, applicationID string) {
	n.logger.InfoContext(ctx, "notify: mentee graduated", "application_id", applicationID)
}

// NotifyMenteeTaskAssigned logs a task-assigned event.
func (n *LogNotifier) NotifyMenteeTaskAssigned(ctx context.Context, taskID string) {
	n.logger.InfoContext(ctx, "notify: mentee task assigned", "task_id", taskID)
}
