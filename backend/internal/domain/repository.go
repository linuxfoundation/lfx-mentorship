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
	List(ctx context.Context, filter models.UserFilter) ([]*models.User, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.UserCreateInput) (*models.User, error)
	Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error)
	Delete(ctx context.Context, id string) error
}

// UserProfileRepository defines persistence operations for user profiles.
type UserProfileRepository interface {
	GetByID(ctx context.Context, id string) (*models.UserProfile, error)
	GetBySlug(ctx context.Context, slug string) (*models.UserProfile, error)
	List(ctx context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, error)
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

// ProgramRepository defines persistence operations for programs and related sub-resources.
type ProgramRepository interface {
	GetByID(ctx context.Context, id string) (*models.Program, error)
	GetBySlug(ctx context.Context, slug string) (*models.Program, error)
	List(ctx context.Context, filter models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error)
	ListCatalog(ctx context.Context, filter models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error)
	GetCatalog(ctx context.Context, id string) (*models.ProgramCatalogItem, error)
	ListCatalogMentees(ctx context.Context, programID string) ([]*models.ProgramCatalogMentee, error)
	Create(ctx context.Context, input models.ProgramCreateInput) (*models.Program, error)
	Update(ctx context.Context, id string, input models.ProgramUpdateInput) (*models.Program, error)
	Delete(ctx context.Context, id string) error

	// Skills
	ListSkills(ctx context.Context, programID string) ([]*models.ProgramSkill, error)
	AddSkill(ctx context.Context, programID string, input models.ProgramSkillCreateInput) (*models.ProgramSkill, error)
	DeleteSkill(ctx context.Context, skillID string) error

	// Funding stats
	GetFundingStats(ctx context.Context, programID string) (*models.ProgramFundingStats, error)
}

// ProgramTermRepository defines persistence operations for program terms.
type ProgramTermRepository interface {
	GetByID(ctx context.Context, id string) (*models.ProgramTerm, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.ProgramTermCreateInput) (*models.ProgramTerm, error)
	Update(ctx context.Context, id string, input models.ProgramTermUpdateInput) (*models.ProgramTerm, error)
	Delete(ctx context.Context, id string) error

	// CountOpenTermsByProgram returns the number of terms with status='open' for a program.
	CountOpenTermsByProgram(ctx context.Context, programID string) (int, error)
}

// ProgramMemberRepository defines persistence operations for program members.
type ProgramMemberRepository interface {
	GetByID(ctx context.Context, id string) (*models.ProgramMember, error)
	FindByProgramAndUser(ctx context.Context, programID, userID string) (*models.ProgramMember, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error)
	Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error)
	Update(ctx context.Context, id string, input models.ProgramMemberUpdateInput) (*models.ProgramMember, error)
	Delete(ctx context.Context, id string) error
}

// ApplicationRepository defines persistence operations for applications.
type ApplicationRepository interface {
	GetByID(ctx context.Context, id string) (*models.Application, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	ListByUser(ctx context.Context, userID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	Create(ctx context.Context, programTermID string, input models.ApplicationCreateInput) (*models.Application, error)
	Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error)
	Delete(ctx context.Context, id string) error

	// CountBlockingAppsForProgram returns applications in a non-terminal state across all terms of a program.
	CountBlockingAppsForProgram(ctx context.Context, programID string) (int, error)
	// CountAcceptedByTerm returns the count of accepted/active applications for a term.
	CountAcceptedByTerm(ctx context.Context, termID string) (int, error)
	// FindByTermAndUser returns an application for a specific term and user, or nil.
	FindByTermAndUser(ctx context.Context, termID, userID string) (*models.Application, error)
	// FindCommittedMenteeByUser returns a mentee application in accepted, active, or
	// graduated status for the user, or nil if they may still apply to a program.
	FindCommittedMenteeByUser(ctx context.Context, userID string) (*models.Application, error)
	// BulkDeclineByTerm moves all pending/submitted applications in a term to declined.
	BulkDeclineByTerm(ctx context.Context, termID string) (int, error)
	// ListPastMenteesByTerm returns accepted/graduated application user IDs for a term.
	ListPastMenteesByTerm(ctx context.Context, termID string) ([]*models.Application, error)
}

// TaskRepository defines persistence operations for tasks.
type TaskRepository interface {
	GetByID(ctx context.Context, id string) (*models.Task, error)
	ListByApplication(ctx context.Context, applicationID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	Create(ctx context.Context, applicationID string, input models.TaskCreateInput) (*models.Task, error)
	Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error)
	Delete(ctx context.Context, id string) error

	// CountPrerequisiteTasksByApplication returns (total, complete) prerequisite task counts.
	CountPrerequisiteTasksByApplication(ctx context.Context, applicationID string) (total int, complete int, err error)
}
