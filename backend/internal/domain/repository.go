// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// UserRepository defines persistence operations for users.
type UserRepository interface {
	GetByID(ctx context.Context, id string) (*models.User, error)
	GetByLFID(ctx context.Context, lfid string) (*models.User, error)
	List(ctx context.Context, filter models.UserFilter) ([]*models.User, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.UserCreateInput) (*models.User, error)
	UpsertByLFID(ctx context.Context, input models.UserCreateInput) (*models.User, error)
	Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error)
	Delete(ctx context.Context, id string) error
}

// ApproverRepository returns the LFIDs in the global mentorship approver team.
type ApproverRepository interface {
	ListLFIDs(ctx context.Context) ([]string, error)
}

// RosterRepository manages platform-authorized FGA rosters.
type RosterRepository interface {
	ListApprovers(ctx context.Context) ([]*models.RosterMember, error)
	AddApprover(ctx context.Context, userID string) (*models.RosterMember, error)
	RemoveApprover(ctx context.Context, userID string) error
}

// UserProfileRepository defines persistence operations for user profiles.
type UserProfileRepository interface {
	GetByID(ctx context.Context, id string) (*models.UserProfile, error)
	GetBySlug(ctx context.Context, slug string) (*models.UserProfile, error)
	List(ctx context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, error)
	UpsertByUserAndType(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, bool, error)
	Update(ctx context.Context, id string, input models.UserProfileUpdateInput) (*models.UserProfile, error)
	Delete(ctx context.Context, id string) error

	// CountActiveMenteeProfiles returns the count of non-deleted mentee profiles for a user.
	CountActiveMenteeProfiles(ctx context.Context, userID string) (int, error)
}

// MenteeRepository defines public directory reads for mentees.
type MenteeRepository interface {
	List(ctx context.Context, filter models.MenteeFilter) (*models.MenteePage, error)
	Summary(ctx context.Context) (*models.MenteeSummary, error)
	GetByUserID(ctx context.Context, userID string) (*models.MenteeDetail, error)
}

// MentorRepository defines public directory reads for mentors.
type MentorRepository interface {
	List(ctx context.Context, filter models.MentorFilter) (*models.MentorPage, error)
	Summary(ctx context.Context) (*models.MentorSummary, error)
	GetByUserID(ctx context.Context, userID string) (*models.MentorDetail, error)
}

// PlatformSummaryRepository defines the read that backs the public
// landing/marketing summary counts.
type PlatformSummaryRepository interface {
	Summary(ctx context.Context) (*models.PlatformSummary, error)
}

// ProgramRepository defines persistence operations for programs and related sub-resources.
type ProgramRepository interface {
	GetByID(ctx context.Context, id string) (*models.Program, error)
	GetBySlug(ctx context.Context, slug string) (*models.Program, error)
	List(ctx context.Context, filter models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error)
	GetEnrollmentTemplate(ctx context.Context, programID string) (*models.ProgramEnrollmentTemplate, error)
	GetManagementSummary(ctx context.Context, programID string) (*models.ProgramManagementSummary, error)
	GetHeaderProjection(ctx context.Context, programID string) (*models.ProgramHeaderProjection, error)
	NameAvailable(ctx context.Context, name, excludeProgramID string) (bool, error)
	ListCatalog(ctx context.Context, filter models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error)
	GetCatalog(ctx context.Context, id string) (*models.ProgramCatalogItem, error)
	ListCatalogMentees(ctx context.Context, programID string) ([]*models.ProgramCatalogMentee, error)
	// ListAdministeredByUser returns the programs userID is an active program admin of, by name.
	ListAdministeredByUser(ctx context.Context, userID string, filter models.AdministeredProgramFilter) ([]*models.AdministeredProgram, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.ProgramCreateInput) (*models.Program, error)
	CreateEnrollment(ctx context.Context, input models.ProgramEnrollmentInput) (*models.Program, error)
	Update(ctx context.Context, id string, input models.ProgramUpdateInput) (*models.Program, error)
	Delete(ctx context.Context, id string) error

	// Skills
	ListSkills(ctx context.Context, programID string) ([]*models.ProgramSkill, error)
	AddSkill(ctx context.Context, programID string, input models.ProgramSkillCreateInput) (*models.ProgramSkill, error)
	DeleteSkill(ctx context.Context, programID, skillID string) error

	// Funding stats
	GetFundingStats(ctx context.Context, programID string) (*models.ProgramFundingStats, error)
}

// FundingStatsRepository defines public funding aggregate reads.
type FundingStatsRepository interface {
	GetFundingTotals(ctx context.Context) (amountRaised float64, amountSpent float64, err error)
}

// ProgramTermRepository defines persistence operations for program terms.
type ProgramTermRepository interface {
	GetByID(ctx context.Context, id string) (*models.ProgramTerm, error)
	GetByProgramAndID(ctx context.Context, programID, id string) (*models.ProgramTerm, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error)
	ListManagementByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTermManagementRow, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.ProgramTermCreateInput) (*models.ProgramTerm, error)
	Update(ctx context.Context, id string, input models.ProgramTermUpdateInput) (*models.ProgramTerm, error)
	Delete(ctx context.Context, id string) error
	CloseWithBulkDecline(ctx context.Context, id string) (*models.ProgramTerm, int, error)

	// CountOpenTermsByProgram returns the number of terms with status='open' for a program.
	CountOpenTermsByProgram(ctx context.Context, programID string) (int, error)
}

// ProgramMemberRepository defines persistence operations for program members.
type ProgramMemberRepository interface {
	GetByID(ctx context.Context, id string) (*models.ProgramMember, error)
	FindByProgramAndUser(ctx context.Context, programID, userID string) (*models.ProgramMember, error)
	FindByProgramUserAndType(ctx context.Context, programID, userID string, memberType models.MemberType) (*models.ProgramMember, error)
	FindActiveReviewerByProgramAndUser(ctx context.Context, programID, userID string) (*models.ProgramMember, error)
	FindActiveProgramAdminByProgramAndUser(ctx context.Context, programID, userID string) (*models.ProgramMember, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error)
	ListMentorManagement(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMentorManagementRow, *models.PaginationMeta, error)
	ListByUser(ctx context.Context, userID string, filter models.ProgramMemberFilter) ([]*models.ProgramMembership, *models.PaginationMeta, error)
	Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error)
	Update(ctx context.Context, id string, input models.ProgramMemberUpdateInput) (*models.ProgramMember, error)
	// UpdateIfStatus applies input only while the row's status is still one of
	// from, returning ErrInvalidStateTransition otherwise. The check and the
	// write are atomic, so a status validated by the caller cannot go stale.
	UpdateIfStatus(ctx context.Context, id string, from []models.ProgramMemberStatus, input models.ProgramMemberUpdateInput) (*models.ProgramMember, error)
	Delete(ctx context.Context, id string) error
}

// ApplicationRepository defines persistence operations for applications.
type ApplicationRepository interface {
	GetByID(ctx context.Context, id string) (*models.Application, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramApplicationFilter) ([]*models.ProgramApplicationRow, *models.PaginationMeta, error)
	ListByUser(ctx context.Context, userID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	Create(ctx context.Context, programTermID string, input models.ApplicationCreateInput) (*models.Application, error)
	CreateWithTasks(ctx context.Context, programTermID string, input models.ApplicationCreateInput, tasks []models.TaskCreateInput) (*models.Application, error)
	Reapply(ctx context.Context, oldID, programTermID string, input models.ApplicationCreateInput) (*models.Application, error)
	ReapplyWithTasks(ctx context.Context, oldID, programTermID string, input models.ApplicationCreateInput, tasks []models.TaskCreateInput) (*models.Application, error)
	Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error)
	// MarkTasksSubmitted sets tasks_submitted once every prerequisite task is submitted or complete,
	// and reports whether this call flipped it, so concurrent callers agree on exactly one first flip.
	MarkTasksSubmitted(ctx context.Context, id string) (bool, error)
	Delete(ctx context.Context, id string) error

	// CountBlockingAppsForProgram returns applications in a non-terminal state across all terms of a program.
	CountBlockingAppsForProgram(ctx context.Context, programID string) (int, error)
	// CountAcceptedByTerm returns the count of accepted applications for a term.
	CountAcceptedByTerm(ctx context.Context, termID string) (int, error)
	// CountByTerm returns every application count for a term.
	CountByTerm(ctx context.Context, termID string) (int, error)
	// FindByTermAndUser returns an application for a specific term and user, or nil.
	FindByTermAndUser(ctx context.Context, termID, userID string) (*models.Application, error)
	// CountWithdrawnByTermAndUser returns how many withdrawn applications a user holds for a term.
	CountWithdrawnByTermAndUser(ctx context.Context, termID, userID string) (int, error)
	// BulkDeclineByTerm moves all pending/submitted applications in a term to declined.
	BulkDeclineByTerm(ctx context.Context, termID string) (int, error)
	// ListPastMenteesByTerm returns accepted/graduated application user IDs for a term.
	ListPastMenteesByTerm(ctx context.Context, termID string) ([]*models.Application, error)
}

// FileReplacement points a file column at a freshly written object.
type FileReplacement struct {
	RowID    string
	Previous *string
	Next     string
	// PendingDeletionID is the grace-period entry that reclaims Next if the replace never commits.
	PendingDeletionID string
}

// FileRepository writes the file-locator columns that only the file routes may set.
// Every write is conditional on the column still holding the value the caller read,
// returning ErrConflict when a concurrent write won, and queues the dropped locator for
// deletion in the same transaction.
type FileRepository interface {
	ReplaceProgramLogo(ctx context.Context, r FileReplacement) error
	// ReplaceProfileLogo also aliases the owner's users.avatar_url to the new logo.
	ReplaceProfileLogo(ctx context.Context, r FileReplacement) error
	// ReplaceTaskFile returns ErrStateLocked once the task is complete.
	ReplaceTaskFile(ctx context.Context, r FileReplacement) error
	ClearProgramLogo(ctx context.Context, programID, previous string) error
	// ClearProfileLogo also nulls users.avatar_url while it still aliases the logo.
	ClearProfileLogo(ctx context.Context, profileID, previous string) error
	// ClearTaskFile returns ErrStateLocked unless the task is incomplete or in progress.
	ClearTaskFile(ctx context.Context, taskID, previous string) error
	// IsProfilePubliclyListed reports whether the profile backs a public mentor or mentee directory entry.
	IsProfilePubliclyListed(ctx context.Context, profileID string) (bool, error)
}

// TaskRepository defines persistence operations for tasks.
type TaskRepository interface {
	GetByID(ctx context.Context, id string) (*models.Task, error)
	ListByApplication(ctx context.Context, applicationID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	Create(ctx context.Context, applicationID string, input models.TaskCreateInput) (*models.Task, error)
	Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error)
	Delete(ctx context.Context, id string) error
}
