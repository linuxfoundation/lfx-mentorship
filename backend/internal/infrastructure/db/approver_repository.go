// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ApproverRepository implements global approver membership reads.
type ApproverRepository struct {
	pool *pgxpool.Pool
}

// NewApproverRepository creates an approver repository.
func NewApproverRepository(pool *pgxpool.Pool) *ApproverRepository {
	return &ApproverRepository{pool: pool}
}

// ListLFIDs returns the current approver roster as LFIDs.
func (r *ApproverRepository) ListLFIDs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.lfid
		FROM mentorship_approver_team_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE u.lfid IS NOT NULL AND u.lfid <> ''`)
	if err != nil {
		return nil, fmt.Errorf("list approver LFIDs: %w", err)
	}
	defer rows.Close()

	lfids := make([]string, 0)
	for rows.Next() {
		var lfid string
		if err := rows.Scan(&lfid); err != nil {
			return nil, fmt.Errorf("scan approver LFID: %w", err)
		}
		lfids = append(lfids, lfid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approver LFIDs: %w", err)
	}
	return lfids, nil
}
