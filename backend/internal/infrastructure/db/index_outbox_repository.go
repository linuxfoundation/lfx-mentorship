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
	_, err := r.pool.Exec(ctx, `INSERT INTO index_outbox (object_type, object_uid, action, headers, data, indexing_config) VALUES ($1,$2,$3,$4,$5,$6)`, record.ObjectType, record.ObjectUID, record.Action, record.Headers, record.Data, record.IndexingConfig)
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
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `WITH claimed AS (SELECT id FROM index_outbox WHERE state = 'pending' ORDER BY created_on FOR UPDATE SKIP LOCKED LIMIT $1) UPDATE index_outbox o SET state = 'in_flight', attempts = attempts + 1 FROM claimed WHERE o.id = claimed.id RETURNING o.id, o.object_type, o.object_uid, o.action, o.headers, o.data, o.indexing_config, o.attempts, o.created_on`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim index outbox: %w", err)
	}
	defer rows.Close()
	result := []domain.IndexOutboxRecord{}
	for rows.Next() {
		var record domain.IndexOutboxRecord
		if err := rows.Scan(&record.ID, &record.ObjectType, &record.ObjectUID, &record.Action, &record.Headers, &record.Data, &record.IndexingConfig, &record.Attempts, &record.CreatedOn); err != nil {
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

func (r *IndexOutboxRepository) MarkSent(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE index_outbox SET state = 'sent', sent_on = NOW() WHERE id = $1`, id)
	return err
}
func (r *IndexOutboxRepository) MarkRetry(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE index_outbox SET state = 'pending' WHERE id = $1`, id)
	return err
}

var _ domain.IndexOutboxRepository = (*IndexOutboxRepository)(nil)
