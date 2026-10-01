// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "context"

// Notifier is a swap point for notification delivery (email, events, etc.).
type Notifier interface {
	NotifyMentorInvited(ctx context.Context, programID, userID, token string)
	// NotifyMentorDeclined tells the mentor a program admin declined them.
	NotifyMentorDeclined(ctx context.Context, programID, userID string)
	// NotifyAdminMentorDeclined tells program admins the mentor declined their invite.
	NotifyAdminMentorDeclined(ctx context.Context, programID, userID string)
	// NotifyAdminMentorAccepted tells program admins the mentor accepted their invite.
	NotifyAdminMentorAccepted(ctx context.Context, programID, userID string)
	NotifyAdminTasksSubmitted(ctx context.Context, applicationID string)
	NotifyMenteeAccepted(ctx context.Context, applicationID, attendanceType string)
}
