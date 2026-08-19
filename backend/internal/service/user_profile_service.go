// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var userProfileSvcTracer = otel.Tracer("user-profiles-service")

// UserProfileService orchestrates user profile reads and writes.
type UserProfileService struct {
	repo domain.UserProfileRepository
}

// NewUserProfileService returns a UserProfileService.
func NewUserProfileService(repo domain.UserProfileRepository) *UserProfileService {
	return &UserProfileService{repo: repo}
}

// GetByID returns the user profile with the given ID.
func (s *UserProfileService) GetByID(ctx context.Context, id string) (*models.UserProfile, error) {
	ctx, span := userProfileSvcTracer.Start(ctx, "UserProfileService.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", id))

	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get user profile: %w", err)
	}
	return p, nil
}

// GetBySlug returns the user profile with the given slug.
func (s *UserProfileService) GetBySlug(ctx context.Context, slug string) (*models.UserProfile, error) {
	ctx, span := userProfileSvcTracer.Start(ctx, "UserProfileService.GetBySlug")
	defer span.End()
	span.SetAttributes(attribute.String("profile.slug", slug))

	p, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get user profile by slug: %w", err)
	}
	return p, nil
}

// List returns a paginated list of user profiles.
func (s *UserProfileService) List(ctx context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error) {
	ctx, span := userProfileSvcTracer.Start(ctx, "UserProfileService.List")
	defer span.End()

	profiles, meta, err := s.repo.List(ctx, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list user profiles: %w", err)
	}
	return profiles, meta, nil
}

// Create validates input and creates a user profile.
func (s *UserProfileService) Create(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, error) {
	ctx, span := userProfileSvcTracer.Start(ctx, "UserProfileService.Create")
	defer span.End()

	if input.ID == "" {
		return nil, fmt.Errorf("%w: id is required", domain.ErrInvalidInput)
	}
	if input.UserID == "" {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	}
	if input.ProfileType == "" {
		return nil, fmt.Errorf("%w: profile_type is required", domain.ErrInvalidInput)
	}
	if input.ProfileType != "mentor" && input.ProfileType != "apprentice" {
		return nil, fmt.Errorf("%w: profile_type must be mentor or apprentice", domain.ErrInvalidInput)
	}

	p, err := s.repo.Create(ctx, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create user profile: %w", err)
	}
	return p, nil
}

// Update applies changes to the user profile with the given ID.
func (s *UserProfileService) Update(ctx context.Context, id string, input models.UserProfileUpdateInput) (*models.UserProfile, error) {
	ctx, span := userProfileSvcTracer.Start(ctx, "UserProfileService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", id))

	p, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update user profile: %w", err)
	}
	return p, nil
}

// Delete removes the user profile with the given ID.
func (s *UserProfileService) Delete(ctx context.Context, id string) error {
	ctx, span := userProfileSvcTracer.Start(ctx, "UserProfileService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", id))

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete user profile: %w", err)
	}
	return nil
}
