// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

type IndexOutboxRepository struct{ pool *pgxpool.Pool }

func NewIndexOutboxRepository(pool *pgxpool.Pool) *IndexOutboxRepository {
	return &IndexOutboxRepository{pool: pool}
}

func (r *IndexOutboxRepository) Enqueue(ctx context.Context, record domain.IndexOutboxRecord) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO index_outbox (object_type, object_uid, action, headers, data, indexing_config) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (object_type, object_uid) DO UPDATE SET action = EXCLUDED.action, headers = EXCLUDED.headers, data = EXCLUDED.data, indexing_config = EXCLUDED.indexing_config, generation = index_outbox.generation + 1, state = CASE WHEN index_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END, attempts = 0`, record.ObjectType, record.ObjectUID, record.Action, record.Headers, record.Data, record.IndexingConfig)
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
	command, err := r.pool.Exec(ctx, `UPDATE index_outbox SET state = 'sent', sent_on = NOW() WHERE id = $1 AND state = 'in_flight' AND generation = $2 AND claimed_generation = $2`, record.ID, record.Generation)
	return command.RowsAffected() == 1, err
}
func (r *IndexOutboxRepository) MarkRetry(ctx context.Context, record domain.IndexOutboxRecord) (bool, error) {
	command, err := r.pool.Exec(ctx, `UPDATE index_outbox SET state = 'pending', claimed_generation = NULL, claimed_at = NULL WHERE id = $1 AND state = 'in_flight' AND generation = $2 AND claimed_generation = $2`, record.ID, record.Generation)
	return command.RowsAffected() == 1, err
}

var _ domain.IndexOutboxRepository = (*IndexOutboxRepository)(nil)
