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

// UserProfileService orchestrates user profile reads.
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
