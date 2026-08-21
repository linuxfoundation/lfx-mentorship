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

var userTracer = otel.Tracer("users-db")

// UserRepository implements domain.UserRepository against PostgreSQL.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// GetByID returns the user with the given UUID or ErrUserNotFound.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
	ctx, span := userTracer.Start(ctx, "db.users.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("db.user_id", id))

	const q = `
		SELECT id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on
		FROM users WHERE id = $1`

	var u models.User
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// List returns a paginated slice of users, optionally filtered by a search string.
func (r *UserRepository) List(ctx context.Context, filter models.UserFilter) ([]*models.User, *models.PaginationMeta, error) {
	ctx, span := userTracer.Start(ctx, "db.users.List")
	defer span.End()

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var total int
	countQ := `SELECT COUNT(*) FROM users`
	args := []any{}
	if filter.Search != "" {
		countQ += ` WHERE name ILIKE $1 OR email ILIKE $1 OR lfid ILIKE $1`
		args = append(args, "%"+filter.Search+"%")
	}
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count users: %w", err)
	}

	listQ := `
		SELECT id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on
		FROM users`
	if filter.Search != "" {
		listQ += ` WHERE name ILIKE $1 OR email ILIKE $1 OR lfid ILIKE $1`
		listQ += fmt.Sprintf(` ORDER BY created_on DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	} else {
		listQ += fmt.Sprintf(` ORDER BY created_on DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn,
		); err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	if users == nil {
		users = []*models.User{}
	}
	return users, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// Create inserts a new user and returns the persisted record.
func (r *UserRepository) Create(ctx context.Context, input models.UserCreateInput) (*models.User, error) {
	ctx, span := userTracer.Start(ctx, "db.users.Create")
	defer span.End()

	const q = `
		INSERT INTO users (id, email, lfid, name, given_name, family_name, avatar_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on`

	var u models.User
	err := r.pool.QueryRow(ctx, q,
		input.ID, input.Email, input.LFID, input.Name, input.GivenName, input.FamilyName, input.AvatarURL,
	).Scan(&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

// Update patches updatable fields for the given user and returns the updated record.
func (r *UserRepository) Update(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
	ctx, span := userTracer.Start(ctx, "db.users.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.user_id", id))

	const q = `
		UPDATE users SET
			email       = COALESCE($2, email),
			lfid        = COALESCE($3, lfid),
			name        = COALESCE($4, name),
			given_name  = COALESCE($5, given_name),
			family_name = COALESCE($6, family_name),
			avatar_url  = COALESCE($7, avatar_url)
		WHERE id = $1
		RETURNING id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on`

	var u models.User
	err := r.pool.QueryRow(ctx, q,
		id, input.Email, input.LFID, input.Name, input.GivenName, input.FamilyName, input.AvatarURL,
	).Scan(&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update user: %w", err)
	}
	return &u, nil
}

// Delete removes the user with the given ID. Returns ErrUserNotFound when absent.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	ctx, span := userTracer.Start(ctx, "db.users.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("db.user_id", id))

	cmd, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete user: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}
