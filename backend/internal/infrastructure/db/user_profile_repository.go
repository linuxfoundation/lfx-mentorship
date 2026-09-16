// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var userProfileTracer = otel.Tracer("user-profiles-db")

// UserProfileRepository implements domain.UserProfileRepository against PostgreSQL.
type UserProfileRepository struct {
	pool *pgxpool.Pool
}

// NewUserProfileRepository creates a new UserProfileRepository.
func NewUserProfileRepository(pool *pgxpool.Pool) *UserProfileRepository {
	return &UserProfileRepository{pool: pool}
}

const userProfileCols = `
	id, user_id, profile_type, slug, first_name, last_name, email, phone,
	logo_url, introduction, terms_and_conditions, number_of_projects,
	address, demographics, socioeconomics, skill_set, profile_links,
	created_on, updated_on`

func scanUserProfile(row pgx.Row) (*models.UserProfile, error) {
	var p models.UserProfile
	err := row.Scan(
		&p.ID, &p.UserID, &p.ProfileType, &p.Slug, &p.FirstName, &p.LastName,
		&p.Email, &p.Phone, &p.LogoURL, &p.Introduction, &p.TermsAndConditions,
		&p.NumberOfProjects, &p.Address, &p.Demographics, &p.Socioeconomics,
		&p.SkillSet, &p.ProfileLinks, &p.CreatedOn, &p.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetByID returns the user profile with the given UUID or ErrUserProfileNotFound.
func (r *UserProfileRepository) GetByID(ctx context.Context, id string) (*models.UserProfile, error) {
	ctx, span := userProfileTracer.Start(ctx, "db.user_profiles.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("db.profile_id", id))

	q := `SELECT` + userProfileCols + ` FROM user_profiles WHERE id = $1`
	p, err := scanUserProfile(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserProfileNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get user profile by id: %w", err)
	}
	return p, nil
}

// GetBySlug returns the user profile with the given slug or ErrUserProfileNotFound.
func (r *UserProfileRepository) GetBySlug(ctx context.Context, slug string) (*models.UserProfile, error) {
	ctx, span := userProfileTracer.Start(ctx, "db.user_profiles.GetBySlug")
	defer span.End()
	span.SetAttributes(attribute.String("db.slug", slug))

	q := `SELECT` + userProfileCols + ` FROM user_profiles WHERE slug = $1`
	p, err := scanUserProfile(r.pool.QueryRow(ctx, q, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserProfileNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get user profile by slug: %w", err)
	}
	return p, nil
}

// nilIfEmpty returns nil when b has zero length (avoids overwriting JSONB with an empty slice).
func nilIfEmpty(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
