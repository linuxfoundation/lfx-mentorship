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

// List returns a paginated slice of user profiles, optionally filtered.
func (r *UserProfileRepository) List(ctx context.Context, filter models.UserProfileFilter) ([]*models.UserProfile, *models.PaginationMeta, error) {
	ctx, span := userProfileTracer.Start(ctx, "db.user_profiles.List")
	defer span.End()

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{}
	where := ` WHERE 1=1`
	if filter.UserID != "" {
		args = append(args, filter.UserID)
		where += fmt.Sprintf(` AND user_id = $%d`, len(args))
	}
	if filter.ProfileType != "" {
		args = append(args, filter.ProfileType)
		where += fmt.Sprintf(` AND profile_type = $%d`, len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_profiles`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count user profiles: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT` + userProfileCols + ` FROM user_profiles` + where +
		fmt.Sprintf(` ORDER BY created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list user profiles: %w", err)
	}
	defer rows.Close()

	var profiles []*models.UserProfile
	for rows.Next() {
		p, err := scanUserProfile(rows)
		if err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan user profile: %w", err)
		}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	if profiles == nil {
		profiles = []*models.UserProfile{}
	}
	return profiles, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// Create inserts a new user profile and returns the persisted record.
func (r *UserProfileRepository) Create(ctx context.Context, input models.UserProfileCreateInput) (*models.UserProfile, error) {
	ctx, span := userProfileTracer.Start(ctx, "db.user_profiles.Create")
	defer span.End()

	const q = `
		INSERT INTO user_profiles (
			id, user_id, profile_type, slug, first_name, last_name, email, phone,
			logo_url, introduction, terms_and_conditions, number_of_projects,
			address, demographics, socioeconomics, skill_set, profile_links
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING` + userProfileCols

	p, err := scanUserProfile(r.pool.QueryRow(ctx, q,
		input.ID, input.UserID, input.ProfileType, input.Slug, input.FirstName, input.LastName,
		input.Email, input.Phone, input.LogoURL, input.Introduction, input.TermsAndConditions,
		input.NumberOfProjects, input.Address, input.Demographics, input.Socioeconomics,
		input.SkillSet, input.ProfileLinks,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create user profile: %w", err)
	}
	return p, nil
}

// Update patches the user profile fields that are set in input.
func (r *UserProfileRepository) Update(ctx context.Context, id string, input models.UserProfileUpdateInput) (*models.UserProfile, error) {
	ctx, span := userProfileTracer.Start(ctx, "db.user_profiles.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.profile_id", id))

	// Build a minimal COALESCE update; only fields provided in input override DB.
	const q = `
		UPDATE user_profiles SET
			slug                = COALESCE($2, slug),
			first_name          = COALESCE($3, first_name),
			last_name           = COALESCE($4, last_name),
			email               = COALESCE($5, email),
			phone               = COALESCE($6, phone),
			logo_url            = COALESCE($7, logo_url),
			introduction        = COALESCE($8, introduction),
			terms_and_conditions= COALESCE($9, terms_and_conditions),
			number_of_projects  = COALESCE($10, number_of_projects),
			address             = COALESCE($11, address),
			demographics        = COALESCE($12, demographics),
			socioeconomics      = COALESCE($13, socioeconomics),
			skill_set           = COALESCE($14, skill_set),
			profile_links       = COALESCE($15, profile_links)
		WHERE id = $1
		RETURNING` + userProfileCols

	p, err := scanUserProfile(r.pool.QueryRow(ctx, q,
		id, input.Slug, input.FirstName, input.LastName, input.Email, input.Phone,
		input.LogoURL, input.Introduction, input.TermsAndConditions, input.NumberOfProjects,
		nilIfEmpty(input.Address), nilIfEmpty(input.Demographics), nilIfEmpty(input.Socioeconomics),
		nilIfEmpty(input.SkillSet), nilIfEmpty(input.ProfileLinks),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserProfileNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update user profile: %w", err)
	}
	return p, nil
}

// Delete removes the user profile with the given ID.
func (r *UserProfileRepository) Delete(ctx context.Context, id string) error {
	ctx, span := userProfileTracer.Start(ctx, "db.user_profiles.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("db.profile_id", id))

	cmd, err := r.pool.Exec(ctx, `DELETE FROM user_profiles WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete user profile: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrUserProfileNotFound
	}
	return nil
}

// nilIfEmpty returns nil when b has zero length (avoids overwriting JSONB with an empty slice).
func nilIfEmpty(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
