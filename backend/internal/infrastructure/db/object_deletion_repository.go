// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

// objectDeletionClaimTimeout is how long an in_flight entry is left to its relay before another may reclaim it.
const objectDeletionClaimTimeout = 5 * time.Minute

// ObjectDeletionRepository implements domain.ObjectDeletionRepository against PostgreSQL.
type ObjectDeletionRepository struct {
	pool        *pgxpool.Pool
	maxAttempts int
	retryDelay  time.Duration
}

// NewObjectDeletionRepository creates a new ObjectDeletionRepository.
func NewObjectDeletionRepository(pool *pgxpool.Pool) *ObjectDeletionRepository {
	return &ObjectDeletionRepository{pool: pool, maxAttempts: 10, retryDelay: time.Minute}
}

// Schedule implements domain.ObjectDeletionRepository.
func (r *ObjectDeletionRepository) Schedule(ctx context.Context, bucket domain.ObjectBucket, locator string, delay time.Duration) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO object_deletions (bucket, locator, next_attempt_at)
		VALUES ($1, $2, NOW() + make_interval(secs => $3))
		RETURNING id`, bucket, locator, delay.Seconds()).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("schedule object deletion: %w", err)
	}
	return id, nil
}

// Claim implements domain.ObjectDeletionRepository. An in_flight entry whose relay
// died is reclaimed after objectDeletionClaimTimeout.
func (r *ObjectDeletionRepository) Claim(ctx context.Context, buckets []domain.ObjectBucket, limit int) ([]domain.ObjectDeletion, error) {
	bucketNames := make([]string, len(buckets))
	for i, b := range buckets {
		bucketNames[i] = string(b)
	}
	rows, err := r.pool.Query(ctx, `
		WITH claimed AS (
			SELECT id FROM object_deletions
			WHERE bucket = ANY($3)
			  AND ((state = 'pending' AND next_attempt_at <= NOW())
			   OR (state = 'in_flight' AND claimed_at < NOW() - make_interval(secs => $2)))
			ORDER BY next_attempt_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE object_deletions d
		SET state = 'in_flight', claimed_at = NOW(), attempts = d.attempts + 1
		FROM claimed
		WHERE d.id = claimed.id
		RETURNING d.id, d.bucket, d.locator, d.attempts, d.claimed_at`, limit, objectDeletionClaimTimeout.Seconds(), bucketNames)
	if err != nil {
		return nil, fmt.Errorf("claim object deletions: %w", err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.ObjectDeletion, error) {
		var e domain.ObjectDeletion
		err := row.Scan(&e.ID, &e.Bucket, &e.Locator, &e.Attempts, &e.ClaimedAt)
		return e, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan claimed object deletions: %w", err)
	}
	return entries, nil
}

// IsReferenced implements domain.ObjectDeletionRepository. Migrated rows can share a
// locator, since legacy program creation could inherit another program's logo.
func (r *ObjectDeletionRepository) IsReferenced(ctx context.Context, locator string) (bool, error) {
	var referenced bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM programs WHERE logo_url = $1)
		    OR EXISTS (SELECT 1 FROM user_profiles WHERE logo_url = $1)
		    OR EXISTS (SELECT 1 FROM users WHERE avatar_url = $1)
		    OR EXISTS (SELECT 1 FROM tasks WHERE file = $1)
		    OR EXISTS (SELECT 1 FROM quarantined_tasks WHERE file = $1)`, locator).Scan(&referenced)
	if err != nil {
		return false, fmt.Errorf("check object deletion references: %w", err)
	}
	return referenced, nil
}

// MarkDone implements domain.ObjectDeletionRepository.
func (r *ObjectDeletionRepository) MarkDone(ctx context.Context, entry domain.ObjectDeletion) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE object_deletions
		SET state = 'done', completed_on = NOW(), claimed_at = NULL, last_error = NULL
		WHERE id = $1 AND state = 'in_flight' AND claimed_at IS NOT DISTINCT FROM $2`, entry.ID, entry.ClaimedAt)
	if err != nil {
		return fmt.Errorf("mark object deletion done: %w", err)
	}
	return nil
}

// MarkRetry implements domain.ObjectDeletionRepository.
func (r *ObjectDeletionRepository) MarkRetry(ctx context.Context, entry domain.ObjectDeletion, cause error) (bool, error) {
	var state string
	err := r.pool.QueryRow(ctx, `
		UPDATE object_deletions
		SET state = CASE WHEN attempts >= $3 THEN 'dead_letter' ELSE 'pending' END,
		    next_attempt_at = NOW() + make_interval(secs => $4), claimed_at = NULL, last_error = $5
		WHERE id = $1 AND state = 'in_flight' AND claimed_at IS NOT DISTINCT FROM $2
		RETURNING state`, entry.ID, entry.ClaimedAt, r.maxAttempts, r.retryDelay.Seconds(), cause.Error()).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("mark object deletion retry: %w", err)
	}
	return state == "dead_letter", nil
}

// queueObjectDeletions queues each non-empty locator in tx, due immediately. Callers
// pass every locator a dropped row held; the relay skips any it did not mint.
func queueObjectDeletions(ctx context.Context, tx pgx.Tx, bucket domain.ObjectBucket, locators ...*string) error {
	values := make([]string, 0, len(locators))
	for _, l := range locators {
		if l != nil && *l != "" {
			values = append(values, *l)
		}
	}
	if len(values) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO object_deletions (bucket, locator)
		SELECT $1, locator FROM unnest($2::text[]) AS locator`, bucket, values); err != nil {
		return fmt.Errorf("queue %s object deletions: %w", bucket, err)
	}
	return nil
}

// queueTaskFileDeletions locks the tasks about to be deleted, including those with no file
// yet, so a concurrent file replace either commits first and is read here or misses the row.
func queueTaskFileDeletions(ctx context.Context, tx pgx.Tx, where string, args ...any) error {
	rows, err := tx.Query(ctx, `SELECT file FROM tasks WHERE `+where+` FOR UPDATE`, args...)
	if err != nil {
		return fmt.Errorf("list task files before delete: %w", err)
	}
	files, err := pgx.CollectRows(rows, pgx.RowTo[*string])
	if err != nil {
		return fmt.Errorf("scan task files before delete: %w", err)
	}
	return queueObjectDeletions(ctx, tx, domain.ObjectBucketAttachments, files...)
}

var _ domain.ObjectDeletionRepository = (*ObjectDeletionRepository)(nil)
