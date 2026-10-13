// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// Notifier is a swap point for notification delivery (email, events, etc.).
type Notifier interface {
	// NotifyProgramSubmitted tells program admins their program is in review, and the LF staff
	// review inbox that it awaits a decision. submitterID may be empty when no actor is known.
	NotifyProgramSubmitted(ctx context.Context, programID, submitterID string)
	// NotifyProgramDecided tells program admins their program was published or rejected.
	NotifyProgramDecided(ctx context.Context, programID string, status models.ProgramStatus)
	NotifyMentorInvited(ctx context.Context, programID, userID, token string)
	// NotifyMentorDeclined tells the mentor a program admin declined their request.
	NotifyMentorDeclined(ctx context.Context, programID, userID string)
	// NotifyAdminMentorDeclined tells program admins the mentor declined their invite.
	NotifyAdminMentorDeclined(ctx context.Context, programID, userID string)
	// NotifyAdminMentorAccepted tells program admins the mentor accepted their invite.
	NotifyAdminMentorAccepted(ctx context.Context, programID, userID string)
	// NotifyAdminMentorRequested tells program admins a mentor asked to join their program.
	NotifyAdminMentorRequested(ctx context.Context, programID, userID string)
	// NotifyAdminMentorWithdrew tells program admins a mentor withdrew their request to join.
	NotifyAdminMentorWithdrew(ctx context.Context, programID, userID string)
	NotifyAdminTasksSubmitted(ctx context.Context, applicationID string)
	// NotifyMenteeApplicationReceived confirms a mentee application and lists its prerequisite tasks.
	NotifyMenteeApplicationReceived(ctx context.Context, applicationID string)
	NotifyMenteeAccepted(ctx context.Context, applicationID, attendanceType string)
	// NotifyMenteeDeclined tells the mentee their application was declined.
	NotifyMenteeDeclined(ctx context.Context, applicationID string)
	// NotifyMenteeWithdrawn confirms the withdrawal to the mentee and tells program admins.
	NotifyMenteeWithdrawn(ctx context.Context, applicationID string)
	// NotifyMenteeGraduated sends the mentee a letter certifying they completed the program.
	NotifyMenteeGraduated(ctx context.Context, applicationID string)
	// NotifyMenteeTaskAssigned tells the mentee a reviewer assigned them a task.
	NotifyMenteeTaskAssigned(ctx context.Context, taskID string)
}
