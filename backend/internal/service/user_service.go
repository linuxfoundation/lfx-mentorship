// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package service contains the application service layer.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var userSvcTracer = otel.Tracer("users-service")

// UserService orchestrates user reads and writes.
type UserService struct {
	repo      domain.UserRepository
	directory domain.AccountDirectory
}

// NewUserService returns a UserService.
func NewUserService(repo domain.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// SetAccountDirectory makes Bootstrap fill a new user's email and name from
// the LF directory when the token did not carry them, as lfx-crowdfunding's
// sign-in sync does.
func (s *UserService) SetAccountDirectory(directory domain.AccountDirectory) {
	s.directory = directory
}

// Bootstrap resolves or creates the local user for a Heimdall LFID. Identity
// comes from the token first; the directory fills what the token left out. A
// failed lookup leaves those fields empty and is logged; it never fails the
// sign-in, and the row is completed on a later sign-in (COALESCE on upsert).
func (s *UserService) Bootstrap(ctx context.Context, lfid string, input models.UserUpdateInput) (*models.User, error) {
	if lfid == "" {
		return nil, fmt.Errorf("%w: principal is required", domain.ErrInvalidInput)
	}
	ctx, span := userSvcTracer.Start(ctx, "UserService.Bootstrap")
	defer span.End()

	s.fillFromDirectory(ctx, lfid, &input)
	user, err := s.repo.BootstrapByLFID(ctx, models.UserCreateInput{LFID: &lfid, Email: input.Email, Name: input.Name, GivenName: input.GivenName, FamilyName: input.FamilyName, AvatarURL: input.AvatarURL})
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("bootstrap user: %w", err)
	}
	return user, nil
}

// fillFromDirectory looks up only what is missing, so a token that carried
// the identity costs no directory call.
func (s *UserService) fillFromDirectory(ctx context.Context, lfid string, input *models.UserUpdateInput) {
	if s.directory == nil {
		return
	}
	if input.Name == nil || input.GivenName == nil || input.FamilyName == nil || input.AvatarURL == nil {
		account, err := s.directory.Account(ctx, lfid)
		switch {
		case err == nil:
			fillMissing(&input.Name, account.Name)
			fillMissing(&input.GivenName, account.GivenName)
			fillMissing(&input.FamilyName, account.FamilyName)
			fillMissing(&input.AvatarURL, account.AvatarURL)
		case errors.Is(err, domain.ErrAccountNotFound):
			// Not every principal is an LF account (M2M clients); nothing to fill.
		default:
			slog.WarnContext(ctx, "directory profile lookup failed; bootstrapping without name", "error", err)
		}
	}
	if input.Email == nil {
		email, err := s.directory.PrimaryEmail(ctx, lfid)
		switch {
		case err == nil && email != "":
			input.Email = &email
		case err != nil && !errors.Is(err, domain.ErrAccountNotFound):
			slog.WarnContext(ctx, "directory email lookup failed; bootstrapping without email", "error", err)
		}
	}
}

// fillMissing sets field to value when the field is empty and the value is not.
func fillMissing(field **string, value *string) {
	if *field == nil && value != nil && *value != "" {
		*field = value
	}
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
	if input.Email != nil {
		return nil, fmt.Errorf("%w: email cannot be updated", domain.ErrForbidden)
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
