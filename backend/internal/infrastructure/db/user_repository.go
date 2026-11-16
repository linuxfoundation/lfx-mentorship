// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var userTracer = otel.Tracer("users-db")

const (
	uniqueViolation = "23505"
	usersEmailKey   = "users_email_key"
)

// sqlUserDisplayName is the name to show for users u: the account's name, else the first and last
// name on their profile joined as up. A sign-in without name claims leaves the account's name NULL
// while the profile holds what the user typed.
const sqlUserDisplayName = `NULLIF(TRIM(COALESCE(NULLIF(TRIM(u.name), ''), CONCAT_WS(' ', up.first_name, up.last_name))), '')`

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
	return r.upsertByLFID(ctx, input, emailWrite{})
}

// BootstrapByLFID is UpsertByLFID for the caller's own sign-in, whose email is the verified token
// claim: any other row still holding that address, another LF account's or an imported row
// without an LFID, is stale and gives it up. Without a claim the stored email is kept.
func (r *UserRepository) BootstrapByLFID(ctx context.Context, input models.UserCreateInput) (*models.User, error) {
	return r.upsertByLFID(ctx, input, emailWrite{release: true})
}

// UpsertByLFIDIfUnchanged is UpsertByLFID for an email looked up outside the transaction: it
// replaces the stored email only while the row is as it was read before the lookup. updatedOn is
// the row's updated_on then, nil for no row. The updated_on trigger changes it on every write, so
// an email another sign-in stored meanwhile is kept even when it is the address already stored.
func (r *UserRepository) UpsertByLFIDIfUnchanged(ctx context.Context, input models.UserCreateInput, updatedOn *time.Time) (*models.User, error) {
	return r.upsertByLFID(ctx, input, emailWrite{guarded: true, updatedOn: updatedOn})
}

// lockUserLFID serialises upserts of one LFID. Two first sign-ins would otherwise both find no
// row and both insert, and the second fails on users_email_key before the lfid arbiter applies.
const lockUserLFID = `SELECT pg_advisory_xact_lock(hashtext('users_lfid'), hashtext($1))`

// lockUserEmail serialises writes of one address. The row locks cover only a row that already
// holds it: a write storing the address for the first time could otherwise commit between a
// verified sign-in's release and its upsert, which then fails on users_email_key.
const lockUserEmail = `SELECT pg_advisory_xact_lock(hashtext('users_email'), hashtext($1))`

// emailWrite says how upsertByLFID treats input.Email.
type emailWrite struct {
	// release takes the address back from any other row first.
	release bool
	// guarded replaces the stored email only while the row's updated_on is still updatedOn.
	guarded   bool
	updatedOn *time.Time
}

// upsertByLFID writes input values over stored ones, keeping a stored value where
// input has none.
func (r *UserRepository) upsertByLFID(ctx context.Context, input models.UserCreateInput, email emailWrite) (*models.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin upsert user transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, lockUserLFID, input.LFID); err != nil {
		return nil, fmt.Errorf("lock user lfid: %w", err)
	}
	if input.Email != nil {
		if _, err := tx.Exec(ctx, lockUserEmail, *input.Email); err != nil {
			return nil, fmt.Errorf("lock user email: %w", err)
		}
		// Lock the caller's row and the one holding the address in ID order first, whether or not
		// the address is released: two accounts writing each other's address would otherwise each
		// lock their own row, then deadlock on the other's.
		if _, err := tx.Exec(ctx, `
			SELECT id FROM users WHERE lfid = $2 OR email = $1 ORDER BY id FOR UPDATE`, *input.Email, input.LFID); err != nil {
			return nil, fmt.Errorf("lock users for email write: %w", err)
		}
	}
	if email.release && input.Email != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE users SET email = NULL
			WHERE email = $1 AND lfid IS DISTINCT FROM $2`, *input.Email, input.LFID); err != nil {
			return nil, fmt.Errorf("release stale user email: %w", err)
		}
	}
	var u models.User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (id, email, lfid, name, given_name, family_name, avatar_url)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)
		ON CONFLICT (lfid) DO UPDATE SET
			email = CASE WHEN $7::bool AND users.updated_on IS DISTINCT FROM $8::timestamptz THEN users.email
				ELSE COALESCE($1::text, users.email) END,
			name = COALESCE($3::text, users.name),
			given_name = COALESCE($4::text, users.given_name),
			family_name = COALESCE($5::text, users.family_name),
			avatar_url = COALESCE($6::text, users.avatar_url), updated_on = NOW()
		RETURNING id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on`,
		input.Email, input.LFID, input.Name, input.GivenName, input.FamilyName, input.AvatarURL,
		email.guarded, email.updatedOn,
	).Scan(&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn)
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == usersEmailKey {
		return nil, domain.ErrEmailInUse
	}
	if err != nil {
		return nil, fmt.Errorf("upsert user by LFID: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit upsert user transaction: %w", err)
	}
	return &u, nil
}

// backfillColumns reads a user for the backfill.
const backfillColumns = `id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on`

// UserCursor is a keyset position in the (created_on, id) order ListIncompleteIdentities uses.
type UserCursor struct {
	CreatedOn time.Time
	ID        string
}

// String encodes the cursor as "<RFC3339Nano created_on>/<id>", the form ParseUserCursor reads.
func (c UserCursor) String() string {
	return c.CreatedOn.UTC().Format(time.RFC3339Nano) + "/" + c.ID
}

// ParseUserCursor decodes a cursor printed by UserCursor.String.
func ParseUserCursor(value string) (UserCursor, error) {
	createdOn, id, ok := strings.Cut(value, "/")
	if !ok || id == "" {
		return UserCursor{}, fmt.Errorf("cursor %q: want <created_on>/<id>", value)
	}
	at, err := time.Parse(time.RFC3339Nano, createdOn)
	if err != nil {
		return UserCursor{}, fmt.Errorf("cursor %q: created_on: %w", value, err)
	}
	return UserCursor{CreatedOn: at, ID: id}, nil
}

// ListIncompleteIdentities returns up to limit signed-in users whose row still
// lacks an email or a name field, oldest first, starting after the cursor when
// one is given. It serves the user-backfill command and is not part of the
// domain repository interface.
func (r *UserRepository) ListIncompleteIdentities(ctx context.Context, after *UserCursor, limit int) ([]*models.User, error) {
	ctx, span := userTracer.Start(ctx, "db.users.ListIncompleteIdentities")
	defer span.End()

	var afterCreatedOn *time.Time
	var afterID *string
	if after != nil {
		afterCreatedOn, afterID = &after.CreatedOn, &after.ID
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+backfillColumns+`
		FROM users
		WHERE lfid IS NOT NULL
		  AND (email IS NULL OR name IS NULL OR given_name IS NULL OR family_name IS NULL OR avatar_url IS NULL)
		  AND ($1::timestamptz IS NULL OR (created_on, id) > ($1::timestamptz, $2::uuid))
		ORDER BY created_on, id
		LIMIT $3`, afterCreatedOn, afterID, limit)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list incomplete users: %w", err)
	}
	defer rows.Close()

	users := []*models.User{}
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan incomplete user: %w", err)
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("iterate incomplete users: %w", err)
	}
	return users, nil
}

// FillMissingIdentity sets only the identity columns that are NULL on the row
// at write time, so a value stored since the row was read is kept, and never
// touches another row: an email already held elsewhere returns ErrEmailInUse.
// It serves the user-backfill command and is not part of the domain interface.
func (r *UserRepository) FillMissingIdentity(ctx context.Context, id string, input models.UserUpdateInput) (*models.User, error) {
	ctx, span := userTracer.Start(ctx, "db.users.FillMissingIdentity")
	defer span.End()
	span.SetAttributes(attribute.String("db.user_id", id))

	var u models.User
	err := r.pool.QueryRow(ctx, `
		UPDATE users SET
			email = COALESCE(users.email, $2),
			name = COALESCE(users.name, $3),
			given_name = COALESCE(users.given_name, $4),
			family_name = COALESCE(users.family_name, $5),
			avatar_url = COALESCE(users.avatar_url, $6),
			updated_on = NOW()
		WHERE id = $1
		RETURNING `+backfillColumns,
		id, input.Email, input.Name, input.GivenName, input.FamilyName, input.AvatarURL,
	).Scan(&u.ID, &u.Email, &u.LFID, &u.Name, &u.GivenName, &u.FamilyName, &u.AvatarURL, &u.CreatedOn, &u.UpdatedOn)
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == usersEmailKey {
		return nil, domain.ErrEmailInUse
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("fill missing identity: %w", err)
	}
	return &u, nil
}

// SearchCandidates returns up to limit users for the invite typeahead.
func (r *UserRepository) SearchCandidates(ctx context.Context, query string, limit int) ([]*models.User, error) {
	ctx, span := userTracer.Start(ctx, "db.users.SearchCandidates")
	defer span.End()

	pattern := likeEscaper.Replace(query)
	rows, err := r.pool.Query(ctx, `
		SELECT id, email, lfid, name, given_name, family_name, avatar_url, created_on, updated_on
		FROM users
		WHERE lfid IS NOT NULL
		  AND (name ILIKE '%' || $1 || '%' OR lfid ILIKE $1 || '%')
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
		args = append(args, containsPattern(filter.Search))
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
