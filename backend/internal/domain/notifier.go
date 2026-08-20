// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "context"

// Notifier is a swap point for notification delivery (email, events, etc.).
type Notifier interface {
	NotifyMentorInvited(ctx context.Context, programID, userID, token string)
	NotifyMentorDeclined(ctx context.Context, programID, userID string)
	NotifyAdminTasksSubmitted(ctx context.Context, applicationID string)
	NotifyMenteeAccepted(ctx context.Context, applicationID, attendanceType string)
}
