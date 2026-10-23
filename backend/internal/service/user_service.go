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
// comes from the token and the body first; the LF directory fills only what
// those and the stored row all lack, so a directory value never replaces an
// edit made through PATCH /me. A failed lookup leaves those fields empty and
// is logged; it never fails the sign-in, and a later sign-in completes the row.
//
// A token email is verified at the gateway, so it reclaims the address from
// any stale row (BootstrapByLFID). The directory does not report whether the
// primary email is verified, so an address from there is stored only while no
// other row holds it: on conflict it is left unset rather than taken.
func (s *UserService) Bootstrap(ctx context.Context, lfid string, input models.UserUpdateInput) (*models.User, error) {
	if lfid == "" {
		return nil, fmt.Errorf("%w: principal is required", domain.ErrInvalidInput)
	}
	ctx, span := userSvcTracer.Start(ctx, "UserService.Bootstrap")
	defer span.End()

	write := models.UserCreateInput{LFID: &lfid, Email: input.Email, Name: input.Name, GivenName: input.GivenName, FamilyName: input.FamilyName, AvatarURL: input.AvatarURL}
	emailFromDirectory := false
	if s.directory != nil {
		known := s.knownIdentity(ctx, lfid, input)
		resolved, err := s.ResolveIdentity(ctx, lfid, known)
		if err != nil {
			slog.WarnContext(ctx, "directory lookup failed; bootstrapping with the token's identity only", "error", err)
		}
		// Only what the directory added is written; stored values are not re-sent.
		fillMissing(&write.Name, newlyFilled(known.Name, resolved.Name))
		fillMissing(&write.GivenName, newlyFilled(known.GivenName, resolved.GivenName))
		fillMissing(&write.FamilyName, newlyFilled(known.FamilyName, resolved.FamilyName))
		fillMissing(&write.AvatarURL, newlyFilled(known.AvatarURL, resolved.AvatarURL))
		if email := newlyFilled(known.Email, resolved.Email); email != nil {
			write.Email, emailFromDirectory = email, true
		}
	}

	var user *models.User
	var err error
	if emailFromDirectory {
		user, err = s.repo.UpsertByLFID(ctx, write)
		if errors.Is(err, domain.ErrEmailInUse) {
			slog.WarnContext(ctx, "directory email already belongs to another user; bootstrapping without email")
			write.Email = nil
			user, err = s.repo.UpsertByLFID(ctx, write)
		}
	} else {
		user, err = s.repo.BootstrapByLFID(ctx, write)
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("bootstrap user: %w", err)
	}
	return user, nil
}

// knownIdentity merges the sign-in input with the stored row, so the directory
// is asked only for fields neither has. A missing row (first sign-in) or a
// failed read leaves the input as is.
func (s *UserService) knownIdentity(ctx context.Context, lfid string, input models.UserUpdateInput) models.UserUpdateInput {
	existing, err := s.repo.GetByLFID(ctx, lfid)
	if err != nil {
		if !errors.Is(err, domain.ErrUserNotFound) {
			slog.WarnContext(ctx, "stored user read failed before directory lookup", "error", err)
		}
		return input
	}
	fillMissing(&input.Email, existing.Email)
	fillMissing(&input.Name, existing.Name)
	fillMissing(&input.GivenName, existing.GivenName)
	fillMissing(&input.FamilyName, existing.FamilyName)
	fillMissing(&input.AvatarURL, existing.AvatarURL)
	return input
}

// ResolveIdentity returns input with the identity fields it lacks filled from
// the LF directory, and writes nothing. Only missing fields are looked up. An
// account the directory does not know (an M2M client, say) is not an error;
// a lookup that fails is returned alongside whatever was filled, so a caller
// can tell "the directory has nothing more" from "the directory did not answer".
func (s *UserService) ResolveIdentity(ctx context.Context, lfid string, input models.UserUpdateInput) (models.UserUpdateInput, error) {
	if s.directory == nil {
		return input, nil
	}
	var lookupErr error
	if input.Name == nil || input.GivenName == nil || input.FamilyName == nil || input.AvatarURL == nil {
		account, err := s.directory.Account(ctx, lfid)
		switch {
		case err == nil:
			fillMissing(&input.Name, account.Name)
			fillMissing(&input.GivenName, account.GivenName)
			fillMissing(&input.FamilyName, account.FamilyName)
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

// newlyFilled returns after when before was empty and after is not: the value
// a resolution step added.
func newlyFilled(before, after *string) *string {
	if before == nil && after != nil && *after != "" {
		return after
	}
	return nil
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
