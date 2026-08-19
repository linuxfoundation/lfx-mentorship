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
}

// ProgramRepository defines persistence operations for programs and related sub-resources.
type ProgramRepository interface {
	GetByID(ctx context.Context, id string) (*models.Program, error)
	GetBySlug(ctx context.Context, slug string) (*models.Program, error)
	List(ctx context.Context, filter models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.ProgramCreateInput) (*models.Program, error)
	Update(ctx context.Context, id string, input models.ProgramUpdateInput) (*models.Program, error)
	Delete(ctx context.Context, id string) error

	// Skills
	ListSkills(ctx context.Context, programID string) ([]*models.ProgramSkill, error)
	AddSkill(ctx context.Context, programID string, input models.ProgramSkillCreateInput) (*models.ProgramSkill, error)
	DeleteSkill(ctx context.Context, skillID string) error

	// Funding stats
	GetFundingStats(ctx context.Context, programID string) (*models.ProgramFundingStats, error)

	// Invitation tokens
	ListInvitationTokens(ctx context.Context, programID string) ([]*models.InvitationToken, error)
	CreateInvitationToken(ctx context.Context, programID string, input models.InvitationTokenCreateInput) (*models.InvitationToken, error)
	DeleteInvitationToken(ctx context.Context, tokenID string) error
}

// ProgramTermRepository defines persistence operations for program terms.
type ProgramTermRepository interface {
	GetByID(ctx context.Context, id string) (*models.ProgramTerm, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error)
	Create(ctx context.Context, input models.ProgramTermCreateInput) (*models.ProgramTerm, error)
	Update(ctx context.Context, id string, input models.ProgramTermUpdateInput) (*models.ProgramTerm, error)
	Delete(ctx context.Context, id string) error
}

// ProgramMemberRepository defines persistence operations for program members.
type ProgramMemberRepository interface {
	GetByID(ctx context.Context, id string) (*models.ProgramMember, error)
	ListByProgram(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error)
	Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error)
	Update(ctx context.Context, id string, input models.ProgramMemberUpdateInput) (*models.ProgramMember, error)
	Delete(ctx context.Context, id string) error

	// Admins
	ListAdminsByProgram(ctx context.Context, programID string) ([]*models.ProgramAdmin, error)
	AddAdmin(ctx context.Context, programID string, input models.ProgramAdminCreateInput) (*models.ProgramAdmin, error)
	DeleteAdmin(ctx context.Context, adminID string) error
}

// ApplicationRepository defines persistence operations for applications.
type ApplicationRepository interface {
	GetByID(ctx context.Context, id string) (*models.Application, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	ListByUser(ctx context.Context, userID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error)
	Create(ctx context.Context, programTermID string, input models.ApplicationCreateInput) (*models.Application, error)
	Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error)
	Delete(ctx context.Context, id string) error
}

// EnrollmentRepository defines persistence operations for enrollments.
type EnrollmentRepository interface {
	GetByID(ctx context.Context, id string) (*models.Enrollment, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.EnrollmentFilter) ([]*models.Enrollment, *models.PaginationMeta, error)
	Create(ctx context.Context, programTermID string, input models.EnrollmentCreateInput) (*models.Enrollment, error)
	Update(ctx context.Context, id string, input models.EnrollmentUpdateInput) (*models.Enrollment, error)
}

// TaskRepository defines persistence operations for tasks.
type TaskRepository interface {
	GetByID(ctx context.Context, id string) (*models.Task, error)
	ListByEnrollment(ctx context.Context, enrollmentID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	ListByProgramTerm(ctx context.Context, programTermID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error)
	Create(ctx context.Context, enrollmentID string, input models.TaskCreateInput) (*models.Task, error)
	Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error)
	Delete(ctx context.Context, id string) error
}
