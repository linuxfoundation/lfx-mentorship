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

// SetAccountDirectory makes Bootstrap refresh the user's name and avatar from
// the LF directory on every sign-in, and fill a missing email from it.
func (s *UserService) SetAccountDirectory(directory domain.AccountDirectory) {
	s.directory = directory
}

// Bootstrap resolves or creates the local user for a Heimdall LFID. The LF
// directory is the source of truth for name and avatar, so on every sign-in
// its current values replace the stored ones; the token and the body supply
// only what the directory does not have. A failed lookup is logged and never
// fails the sign-in: the token's values are stored and the next sign-in
// refreshes the rest.
//
// A token email is verified at the gateway, so it reclaims the address from
// any stale row. The directory does not report whether the primary email is
// verified, so an address from there fills an empty column only, and only
// while no other row holds it: on conflict it is left unset rather than taken.
func (s *UserService) Bootstrap(ctx context.Context, lfid string, input models.UserUpdateInput) (*models.User, error) {
	if lfid == "" {
		return nil, fmt.Errorf("%w: principal is required", domain.ErrInvalidInput)
	}
	if err := validateUserNames(input.Name, input.GivenName, input.FamilyName); err != nil {
		return nil, err
	}
	ctx, span := userSvcTracer.Start(ctx, "UserService.Bootstrap")
	defer span.End()

	write := models.UserCreateInput{LFID: &lfid, Email: input.Email, Name: input.Name, GivenName: input.GivenName, FamilyName: input.FamilyName, AvatarURL: input.AvatarURL}
	var fill models.UserIdentityFill
	if s.directory != nil {
		s.refreshProfile(ctx, lfid, &write)
		if write.Email == nil && !s.hasStoredEmail(ctx, lfid) {
			fill.Email = s.directoryEmail(ctx, lfid)
		}
	}

	user, err := s.repo.BootstrapByLFID(ctx, write, fill)
	if errors.Is(err, domain.ErrEmailInUse) && fill.Email != nil {
		// Only a fill email can collide: the token's was released first.
		slog.WarnContext(ctx, "directory email already belongs to another user; bootstrapping without email")
		fill.Email = nil
		user, err = s.repo.BootstrapByLFID(ctx, write, fill)
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("bootstrap user: %w", err)
	}
	return user, nil
}

// refreshProfile replaces write's name and avatar fields with the directory's
// values wherever the directory has one.
func (s *UserService) refreshProfile(ctx context.Context, lfid string, write *models.UserCreateInput) {
	account, err := s.directory.Account(ctx, lfid)
	switch {
	case err == nil:
		preferValue(&write.Name, boundedName(account.Name))
		preferValue(&write.GivenName, boundedName(account.GivenName))
		preferValue(&write.FamilyName, boundedName(account.FamilyName))
		preferValue(&write.AvatarURL, account.AvatarURL)
	case errors.Is(err, domain.ErrAccountNotFound):
	default:
		slog.WarnContext(ctx, "directory profile lookup failed; keeping the token's profile", "error", err)
	}
}

// hasStoredEmail reports whether the caller's row already has an email. A
// missing row or a failed read counts as none, which costs a directory lookup.
func (s *UserService) hasStoredEmail(ctx context.Context, lfid string) bool {
	existing, err := s.repo.GetByLFID(ctx, lfid)
	if err != nil {
		if !errors.Is(err, domain.ErrUserNotFound) {
			slog.WarnContext(ctx, "stored user read failed before directory lookup", "error", err)
		}
		return false
	}
	return existing.Email != nil && *existing.Email != ""
}

// directoryEmail returns the caller's primary email from the directory, or nil.
func (s *UserService) directoryEmail(ctx context.Context, lfid string) *string {
	email, err := s.directory.PrimaryEmail(ctx, lfid)
	switch {
	case err == nil && email != "":
		return &email
	case err != nil && !errors.Is(err, domain.ErrAccountNotFound):
		slog.WarnContext(ctx, "directory email lookup failed", "error", err)
	}
	return nil
}

// SyncEmail stores the caller's verified token email when it differs from the
// stored one, so notifications follow an address change without waiting for a
// sign-in. It is best-effort: a failure is logged and never fails the request.
func (s *UserService) SyncEmail(ctx context.Context, user *models.User, email string) {
	if email == "" || user == nil || user.LFID == nil || (user.Email != nil && *user.Email == email) {
		return
	}
	// BootstrapByLFID also releases the address from a stale row that still holds it.
	if _, err := s.repo.BootstrapByLFID(ctx, models.UserCreateInput{LFID: user.LFID, Email: &email}, models.UserIdentityFill{}); err != nil {
		slog.WarnContext(ctx, "token email sync failed", "error", err)
	}
}

// ResolveIdentity returns input with the identity fields it lacks filled from
// the LF directory, and writes nothing. Only missing fields are looked up. An
// account the directory does not know (an M2M client, say) is not an error;
// a lookup that fails is returned alongside whatever was filled, so a caller
// can tell "the directory has nothing more" from "the directory did not answer".
// A directory name longer than maxNameLength is left out.
func (s *UserService) ResolveIdentity(ctx context.Context, lfid string, input models.UserUpdateInput) (models.UserUpdateInput, error) {
	if s.directory == nil {
		return input, nil
	}
	var lookupErr error
	if input.Name == nil || input.GivenName == nil || input.FamilyName == nil || input.AvatarURL == nil {
		account, err := s.directory.Account(ctx, lfid)
		switch {
		case err == nil:
			fillMissing(&input.Name, boundedName(account.Name))
			fillMissing(&input.GivenName, boundedName(account.GivenName))
			fillMissing(&input.FamilyName, boundedName(account.FamilyName))
			fillMissing(&input.AvatarURL, account.AvatarURL)
		case errors.Is(err, domain.ErrAccountNotFound):
		default:
			lookupErr = errors.Join(lookupErr, fmt.Errorf("directory profile lookup: %w", err))
		}
	}
	if input.Email == nil {
		email, err := s.directory.PrimaryEmail(ctx, lfid)
		switch {
		case err == nil && email != "":
			input.Email = &email
		case err != nil && !errors.Is(err, domain.ErrAccountNotFound):
			lookupErr = errors.Join(lookupErr, fmt.Errorf("directory email lookup: %w", err))
		}
	}
	return input, lookupErr
}

// fillMissing sets field to value when the field is empty and the value is not.
func fillMissing(field **string, value *string) {
	if *field == nil && value != nil && *value != "" {
		*field = value
	}
}

// preferValue sets field to value when the value is not empty.
func preferValue(field **string, value *string) {
	if value != nil && *value != "" {
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

	var err error
	if filter.Search, err = normalizeSearch(filter.Search); err != nil {
		return nil, nil, err
	}
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
	if err := validateUserNames(input.Name, input.GivenName, input.FamilyName); err != nil {
		return nil, err
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
	if err := validateUserNames(input.Name, input.GivenName, input.FamilyName); err != nil {
		return nil, err
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
