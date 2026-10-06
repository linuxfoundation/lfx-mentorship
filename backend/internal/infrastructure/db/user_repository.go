// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// GetByLFID returns the local user mapped to a Heimdall principal.
func (r *UserRepository) GetByLFID(ctx context.Context, lfid string) (*models.User, error) {
	ctx, span := userTracer.Start(ctx, "db.users.GetByLFID")
	defer span.End()
	var u models.User
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on
		FROM users WHERE lfid = $1`, lfid).Scan(
		&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by LFID: %w", err)
	}
	return &u, nil
}

// UpsertByLFID atomically creates or refreshes the local user for a gateway principal.
func (r *UserRepository) UpsertByLFID(ctx context.Context, input models.UserCreateInput) (*models.User, error) {
	var u models.User
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (id, email, lfid, name, given_name, family_name, avatar_url)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)
		ON CONFLICT (lfid) DO UPDATE SET
			email = COALESCE(EXCLUDED.email, users.email), name = COALESCE(EXCLUDED.name, users.name),
			given_name = COALESCE(EXCLUDED.given_name, users.given_name),
			family_name = COALESCE(EXCLUDED.family_name, users.family_name),
			avatar_url = COALESCE(users.avatar_url, EXCLUDED.avatar_url), updated_on = NOW()
		RETURNING id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on`,
		input.Email, input.LFID, input.Name, input.GivenName, input.FamilyName, input.AvatarURL,
	).Scan(&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn)
	if err != nil {
		return nil, fmt.Errorf("upsert user by LFID: %w", err)
	}
	return &u, nil
}

// SearchCandidates returns up to limit users for the invite typeahead.
func (r *UserRepository) SearchCandidates(ctx context.Context, query string, limit int) ([]*models.User, error) {
	ctx, span := userTracer.Start(ctx, "db.users.SearchCandidates")
	defer span.End()

	pattern := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	rows, err := r.pool.Query(ctx, `
		SELECT id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on
		FROM users
		WHERE lfid IS NOT NULL
		  AND (name ILIKE '%' || $1 || '%' OR lfid ILIKE $1 || '%' OR lower(email) = lower($2))
		ORDER BY lower(lfid) = lower($2) DESC, name NULLS LAST, lfid
		LIMIT $3`, pattern, query, limit)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("search candidate users: %w", err)
	}
	defer rows.Close()

	users := []*models.User{}
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn,
		); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan candidate user: %w", err)
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("search candidate users: %w", err)
	}
	return users, nil
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
// The user's profiles cascade, so their logos and the avatar are queued for deletion.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	ctx, span := userTracer.Start(ctx, "db.users.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("db.user_id", id))

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete user transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Locks the user, then its profiles: the order profile logo writes take (lockProfileOwner).
	var avatarURL *string
	err = tx.QueryRow(ctx, `SELECT avatar_url FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&avatarURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("lock user before delete: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT logo_url FROM user_profiles WHERE user_id = $1 FOR UPDATE`, id)
	if err != nil {
		return fmt.Errorf("list user files before delete: %w", err)
	}
	locators, err := pgx.CollectRows(rows, pgx.RowTo[*string])
	if err != nil {
		return fmt.Errorf("scan user files before delete: %w", err)
	}
	if err := queueObjectDeletions(ctx, tx, domain.ObjectBucketLogos, append(locators, avatarURL)...); err != nil {
		return err
	}

	cmd, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete user: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete user transaction: %w", err)
	}
	return nil
}
