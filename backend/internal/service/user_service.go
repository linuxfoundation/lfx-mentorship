// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package service contains the application service layer.
package service

import (
	"context"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var userSvcTracer = otel.Tracer("users-service")

// UserService orchestrates user reads and writes.
type UserService struct {
	repo domain.UserRepository
}

// Bootstrap resolves or creates the local user for a Heimdall LFID.
func (s *UserService) Bootstrap(ctx context.Context, lfid string, input models.UserUpdateInput) (*models.User, error) {
	if lfid == "" {
		return nil, fmt.Errorf("%w: principal is required", domain.ErrInvalidInput)
	}
	input.LFID = &lfid
	user, err := s.repo.UpsertByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: input.Email, Name: input.Name, GivenName: input.GivenName, FamilyName: input.FamilyName, AvatarURL: input.AvatarURL})
	if err != nil {
		return nil, fmt.Errorf("bootstrap user: %w", err)
	}
	return user, nil
}

// NewUserService returns a UserService.
func NewUserService(repo domain.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// GetByID returns the user with the given ID.
func (s *UserService) GetByID(ctx context.Context, id string) (*models.User, error) {
	ctx, span := userSvcTracer.Start(ctx, "UserService.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", id))

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// List returns a paginated list of users.
func (s *UserService) List(ctx context.Context, filter models.UserFilter) ([]*models.User, *models.PaginationMeta, error) {
	ctx, span := userSvcTracer.Start(ctx, "UserService.List")
	defer span.End()

	users, meta, err := s.repo.List(ctx, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list users: %w", err)
	}
	return users, meta, nil
}

// Create validates input and creates a user.
func (s *UserService) Create(ctx context.Context, input models.UserCreateInput) (*models.User, error) {
	ctx, span := userSvcTracer.Start(ctx, "UserService.Create")
	defer span.End()

	if input.ID == "" {
		return nil, fmt.Errorf("%w: id is required", domain.ErrInvalidInput)
	}

	user, err := s.repo.Create(ctx, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// Update validates and applies changes to the user with the given ID.
func (s *UserService) Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
	ctx, span := userSvcTracer.Start(ctx, "UserService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", id))

	if input.LFID != nil {
		return nil, fmt.Errorf("%w: lfid cannot be updated", domain.ErrForbidden)
	}

	user, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update user: %w", err)
	}
	return user, nil
}

// Delete removes the user with the given ID when the caller owns that user.
func (s *UserService) Delete(ctx context.Context, id, actorID string) error {
	ctx, span := userSvcTracer.Start(ctx, "UserService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", id))

	if actorID == "" || actorID != id {
		return fmt.Errorf("%w: caller may only delete their own user", domain.ErrForbidden)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}
