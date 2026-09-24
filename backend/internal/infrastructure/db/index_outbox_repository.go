// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

type IndexOutboxRepository struct {
	pool        *pgxpool.Pool
	maxAttempts int
}

func NewIndexOutboxRepository(pool *pgxpool.Pool) *IndexOutboxRepository {
	return &IndexOutboxRepository{pool: pool, maxAttempts: 10}
}

func (r *IndexOutboxRepository) SetMaxAttempts(maxAttempts int) {
	if maxAttempts > 0 {
		r.maxAttempts = maxAttempts
	}
}

func (r *IndexOutboxRepository) Enqueue(ctx context.Context, record domain.IndexOutboxRecord) error {
	headers := map[string]string{}
	if len(record.Headers) > 0 {
		if err := json.Unmarshal(record.Headers, &headers); err != nil {
			return fmt.Errorf("enqueue index outbox: decode headers: %w", err)
		}
	}
	sanitizedHeaders, err := json.Marshal(domain.SanitizedIndexHeaders(headers))
	if err != nil {
		return fmt.Errorf("enqueue index outbox: encode sanitized headers: %w", err)
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO index_outbox (object_type, object_uid, action, headers, data, indexing_config) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (object_type, object_uid) DO UPDATE SET action = EXCLUDED.action, headers = EXCLUDED.headers, data = EXCLUDED.data, indexing_config = EXCLUDED.indexing_config, generation = index_outbox.generation + 1, state = CASE WHEN index_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END, claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END, claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END, attempts = 0, sent_on = NULL`, record.ObjectType, record.ObjectUID, record.Action, sanitizedHeaders, record.Data, record.IndexingConfig)
	if err != nil {
		return fmt.Errorf("enqueue index outbox: %w", err)
	}
	return nil
}

func (r *IndexOutboxRepository) Claim(ctx context.Context, limit int) ([]domain.IndexOutboxRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `WITH claimed AS (SELECT id FROM index_outbox WHERE state = 'pending' OR (state = 'in_flight' AND claimed_at < NOW() - INTERVAL '5 minutes') ORDER BY created_on FOR UPDATE SKIP LOCKED LIMIT $1) UPDATE index_outbox o SET state = 'in_flight', attempts = attempts + 1, claimed_generation = generation, claimed_at = NOW() FROM claimed WHERE o.id = claimed.id RETURNING o.id, o.object_type, o.object_uid, o.action, o.headers, o.data, o.indexing_config, o.attempts, o.generation, o.claimed_generation, o.claimed_at, o.created_on`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim index outbox: %w", err)
	}
	defer rows.Close()
	result := []domain.IndexOutboxRecord{}
	for rows.Next() {
		var record domain.IndexOutboxRecord
		if err := rows.Scan(&record.ID, &record.ObjectType, &record.ObjectUID, &record.Action, &record.Headers, &record.Data, &record.IndexingConfig, &record.Attempts, &record.Generation, &record.ClaimedGeneration, &record.ClaimedAt, &record.CreatedOn); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *IndexOutboxRepository) MarkSent(ctx context.Context, record domain.IndexOutboxRecord) (bool, error) {
	const query = `
		WITH acknowledged AS (
			UPDATE index_outbox
			SET state = 'sent', sent_on = NOW()
			WHERE id = $1 AND state = 'in_flight' AND generation = $2
			  AND claimed_generation = $2 AND claimed_at IS NOT DISTINCT FROM $3
			RETURNING id
		), requeued AS (
			UPDATE index_outbox
			SET state = 'pending', claimed_generation = NULL, claimed_at = NULL, sent_on = NULL
			WHERE id = $1 AND state = 'in_flight' AND claimed_generation = $2
			  AND generation > $2 AND claimed_at IS NOT DISTINCT FROM $3
			RETURNING id
		)
		SELECT EXISTS (SELECT 1 FROM acknowledged), EXISTS (SELECT 1 FROM requeued)`
	var acknowledged, requeued bool
	if err := r.pool.QueryRow(ctx, query, record.ID, record.Generation, record.ClaimedAt).Scan(&acknowledged, &requeued); err != nil {
		return false, err
	}
	return acknowledged || requeued, nil
}
func (r *IndexOutboxRepository) MarkRetry(ctx context.Context, record domain.IndexOutboxRecord) (bool, error) {
	command, err := r.pool.Exec(ctx, `UPDATE index_outbox SET state = CASE WHEN attempts + 1 >= $4 THEN 'dead_letter' ELSE 'pending' END, attempts = attempts + 1, claimed_generation = NULL, claimed_at = NULL WHERE id = $1 AND state = 'in_flight' AND generation = $2 AND claimed_generation = $2 AND claimed_at IS NOT DISTINCT FROM $3`, record.ID, record.Generation, record.ClaimedAt, r.maxAttempts)
	return command.RowsAffected() == 1, err
}

// RequeueDeadLetter returns one retained record to the normal relay path.
func (r *IndexOutboxRepository) RequeueDeadLetter(ctx context.Context, objectType, objectUID string) (bool, error) {
	command, err := r.pool.Exec(ctx, `
		UPDATE index_outbox
		SET state = 'pending', claimed_generation = NULL, claimed_at = NULL,
		    attempts = 0, sent_on = NULL
		WHERE state = 'dead_letter' AND object_type = $1 AND object_uid = $2`, objectType, objectUID)
	if err != nil {
		return false, fmt.Errorf("requeue index dead-letter record: %w", err)
	}
	return command.RowsAffected() == 1, nil
}

var _ domain.IndexOutboxRepository = (*IndexOutboxRepository)(nil)
