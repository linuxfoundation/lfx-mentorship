// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"expvar"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

var fgaOutboxStaleDeadLettered = expvar.NewInt("fga_outbox_stale_dead_lettered")

// FGAOutboxRepository implements generation-guarded FGA marker delivery.
type FGAOutboxRepository struct {
	pool        *pgxpool.Pool
	maxAttempts int
}

// NewFGAOutboxRepository creates an FGA outbox repository.
func NewFGAOutboxRepository(pool *pgxpool.Pool) *FGAOutboxRepository {
	return &FGAOutboxRepository{pool: pool, maxAttempts: 10}
}

// SetMaxAttempts bounds stale-claim recovery before dead lettering.
func (r *FGAOutboxRepository) SetMaxAttempts(maxAttempts int) {
	if maxAttempts > 0 {
		r.maxAttempts = maxAttempts
	}
}

// EnqueueObject coalesces a whole-object change and invalidates an older claim.
func (r *FGAOutboxRepository) EnqueueObject(ctx context.Context, objectType, objectUID, operation string) error {
	const query = `
		INSERT INTO fga_outbox (marker_kind, object_type, object_uid, desired_operation)
		VALUES ('object', $1, $2, $3)
		ON CONFLICT (object_type, object_uid) WHERE marker_kind = 'object'
		DO UPDATE SET desired_operation = EXCLUDED.desired_operation,
		              generation = fga_outbox.generation + 1,
		              state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
	              claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
	              claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
		              attempts = 0, next_attempt_at = NOW(), last_error = NULL,
		              updated_on = NOW()`
	if _, err := r.pool.Exec(ctx, query, objectType, objectUID, operation); err != nil {
		return fmt.Errorf("enqueue FGA object marker: %w", err)
	}
	return nil
}

// EnqueueMembership coalesces a relation-member change and invalidates an older claim.
func (r *FGAOutboxRepository) EnqueueMembership(ctx context.Context, objectType, objectUID, relation, username string) error {
	const query = `
		INSERT INTO fga_outbox (marker_kind, object_type, object_uid, relation, username, desired_operation)
		VALUES ('membership', $1, $2, $3, $4, 'sync')
		ON CONFLICT (object_type, object_uid, relation, username) WHERE marker_kind = 'membership'
		DO UPDATE SET generation = fga_outbox.generation + 1,
		              state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
	              claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
	              claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
		              attempts = 0, next_attempt_at = NOW(), last_error = NULL,
		              updated_on = NOW()`
	if _, err := r.pool.Exec(ctx, query, objectType, objectUID, relation, username); err != nil {
		return fmt.Errorf("enqueue FGA membership marker: %w", err)
	}
	return nil
}

func (r *FGAOutboxRepository) EnqueueMembershipRemoval(ctx context.Context, objectType, objectUID, relation, username string) error {
	const query = `
		INSERT INTO fga_outbox (marker_kind, object_type, object_uid, relation, username, desired_operation)
		VALUES ('membership', $1, $2, $3, $4, 'remove')
		ON CONFLICT (object_type, object_uid, relation, username) WHERE marker_kind = 'membership'
		DO UPDATE SET desired_operation = 'remove', generation = fga_outbox.generation + 1,
		              state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
	              claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END,
	              claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
		              attempts = 0, next_attempt_at = NOW(), last_error = NULL, updated_on = NOW()`
	if _, err := r.pool.Exec(ctx, query, objectType, objectUID, relation, username); err != nil {
		return fmt.Errorf("enqueue FGA membership removal: %w", err)
	}
	return nil
}

// Claim marks a bounded batch in flight and returns the claimed generations.
func (r *FGAOutboxRepository) Claim(ctx context.Context, limit int) ([]domain.FGAOutboxMarker, error) {
	if limit <= 0 {
		return []domain.FGAOutboxMarker{}, nil
	}
	var deadLettered int
	if err := r.pool.QueryRow(ctx, `
		WITH stale_dead AS (
			UPDATE fga_outbox
			SET state = 'dead_letter', claimed_generation = NULL, claimed_at = NULL,
			    attempts = attempts + 1, last_error = 'relay crashed while in flight',
			    updated_on = NOW()
			WHERE state = 'in_flight'
			  AND claimed_at <= NOW() - INTERVAL '5 minutes'
			  AND generation = claimed_generation
			  AND attempts + 1 >= $1
			RETURNING id
		)
		SELECT COUNT(*) FROM stale_dead`, r.maxAttempts).Scan(&deadLettered); err != nil {
		return nil, fmt.Errorf("dead-letter stale FGA outbox markers: %w", err)
	}
	if deadLettered > 0 {
		fgaOutboxStaleDeadLettered.Add(int64(deadLettered))
		slog.Default().WarnContext(ctx, "dead-lettered stale FGA outbox markers", "count", deadLettered)
	}
	const query = `
		WITH lockable AS (
			SELECT pending.id, pending.object_type, pending.object_uid
			FROM fga_outbox AS pending
			WHERE ((pending.state = 'pending' AND pending.next_attempt_at <= NOW())
			   OR (pending.state = 'in_flight'
			       AND pending.claimed_at <= NOW() - INTERVAL '5 minutes'
			       AND (pending.generation > pending.claimed_generation OR pending.attempts + 1 < $2)))
			  AND NOT EXISTS (
				SELECT 1 FROM fga_outbox AS active
				WHERE active.object_type = pending.object_type
				  AND active.object_uid = pending.object_uid
				  AND active.state = 'in_flight'
				  AND active.id <> pending.id
			  )
			  AND pg_try_advisory_xact_lock(
				hashtextextended(pending.object_type || ':' || pending.object_uid, 0)
			  )
			ORDER BY pending.object_type, pending.object_uid, pending.id
			FOR UPDATE SKIP LOCKED
		),
		candidates AS (
			SELECT DISTINCT ON (lockable.object_type, lockable.object_uid) lockable.id
			FROM lockable
			ORDER BY lockable.object_type, lockable.object_uid, lockable.id
			LIMIT $1
		)
		UPDATE fga_outbox AS outbox
		SET state = 'in_flight', claimed_generation = outbox.generation,
		    claimed_at = NOW(), next_attempt_at = NOW() + INTERVAL '5 minutes',
		    attempts = CASE
		                   WHEN outbox.state = 'in_flight' AND outbox.generation = outbox.claimed_generation THEN outbox.attempts + 1
		                   ELSE outbox.attempts
		               END,
		    updated_on = NOW()
		FROM candidates
		WHERE outbox.id = candidates.id
		RETURNING outbox.id, outbox.marker_kind, outbox.object_type, outbox.object_uid,
		          outbox.relation, outbox.username, outbox.desired_operation,
			          outbox.generation, outbox.claimed_generation, outbox.claimed_at, outbox.attempts,
		          outbox.last_error`

	rows, err := r.pool.Query(ctx, query, limit, r.maxAttempts)
	if err != nil {
		return nil, fmt.Errorf("claim FGA outbox markers: %w", err)
	}
	defer rows.Close()

	markers := make([]domain.FGAOutboxMarker, 0, limit)
	for rows.Next() {
		var marker domain.FGAOutboxMarker
		if err := rows.Scan(
			&marker.ID, &marker.MarkerKind, &marker.ObjectType, &marker.ObjectUID,
			&marker.Relation, &marker.Username, &marker.DesiredOperation,
			&marker.Generation, &marker.ClaimedGeneration, &marker.ClaimedAt, &marker.Attempts,
			&marker.LastError,
		); err != nil {
			return nil, fmt.Errorf("scan FGA outbox marker: %w", err)
		}
		markers = append(markers, marker)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate FGA outbox markers: %w", err)
	}
	return markers, nil
}

// Acknowledge deletes only the generation that was actually delivered.
func (r *FGAOutboxRepository) Acknowledge(ctx context.Context, marker domain.FGAOutboxMarker) (bool, error) {
	const query = `
		WITH acknowledged AS (
			DELETE FROM fga_outbox
			WHERE id = $1 AND state = 'in_flight' AND claimed_generation = $2
			  AND generation = $2 AND claimed_at IS NOT DISTINCT FROM $3
			RETURNING id
		), requeued AS (
			UPDATE fga_outbox
			SET state = 'pending', claimed_generation = NULL, claimed_at = NULL,
			    next_attempt_at = NOW(), updated_on = NOW()
			WHERE id = $1 AND state = 'in_flight' AND claimed_generation = $2
			  AND generation > $2 AND claimed_at IS NOT DISTINCT FROM $3
			RETURNING id
		)
		SELECT EXISTS (SELECT 1 FROM acknowledged), EXISTS (SELECT 1 FROM requeued)`
	var acknowledged, requeued bool
	if err := r.pool.QueryRow(ctx, query, marker.ID, marker.Generation, marker.ClaimedAt).Scan(&acknowledged, &requeued); err != nil {
		return false, fmt.Errorf("acknowledge FGA outbox marker: %w", err)
	}
	return acknowledged, nil
}

// Retry returns a claimed marker to pending without overwriting a newer generation.
func (r *FGAOutboxRepository) Retry(ctx context.Context, marker domain.FGAOutboxMarker, nextAttemptAt time.Time, errText string) error {
	const query = `
		UPDATE fga_outbox
		SET state = 'pending', claimed_generation = NULL, claimed_at = NULL,
		    attempts = attempts + 1, next_attempt_at = $3, last_error = $4,
		    updated_on = NOW()
		WHERE id = $1 AND state = 'in_flight' AND claimed_generation = $2
		  AND claimed_at IS NOT DISTINCT FROM $5`
	if _, err := r.pool.Exec(ctx, query, marker.ID, marker.Generation, nextAttemptAt, errText, marker.ClaimedAt); err != nil {
		return fmt.Errorf("retry FGA outbox marker: %w", err)
	}
	return nil
}

// DeadLetter stops retrying a marker after the configured attempt limit while
// preserving the marker and failure details for operator inspection.
func (r *FGAOutboxRepository) DeadLetter(ctx context.Context, marker domain.FGAOutboxMarker, errText string) error {
	const query = `
		UPDATE fga_outbox
		SET state = 'dead_letter', claimed_generation = NULL, claimed_at = NULL,
		    attempts = attempts + 1, last_error = $3, updated_on = NOW()
		WHERE id = $1 AND state = 'in_flight' AND claimed_generation = $2
		  AND generation = $2
		  AND claimed_at IS NOT DISTINCT FROM $4`
	if _, err := r.pool.Exec(ctx, query, marker.ID, marker.Generation, errText, marker.ClaimedAt); err != nil {
		return fmt.Errorf("dead-letter FGA outbox marker: %w", err)
	}
	return nil
}

// RequeueDeadLetter returns one exact retained marker to the normal relay path.
func (r *FGAOutboxRepository) RequeueDeadLetter(ctx context.Context, objectType, objectUID, relation, username string) (bool, error) {
	command, err := r.pool.Exec(ctx, `
		UPDATE fga_outbox
		SET state = 'pending', claimed_generation = NULL, claimed_at = NULL,
		    attempts = 0, next_attempt_at = NOW(), last_error = NULL, updated_on = NOW()
		WHERE state = 'dead_letter' AND object_type = $1 AND object_uid = $2
		  AND (
		    ($3 = '' AND $4 = '' AND marker_kind = 'object')
		    OR
		    ($3 <> '' AND $4 <> '' AND marker_kind = 'membership'
		      AND relation = $3 AND username = $4)
		  )`, objectType, objectUID, relation, username)
	if err != nil {
		return false, fmt.Errorf("requeue FGA dead-letter marker: %w", err)
	}
	return command.RowsAffected() == 1, nil
}
