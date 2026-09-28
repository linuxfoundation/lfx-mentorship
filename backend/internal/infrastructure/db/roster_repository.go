// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// RosterRepository implements platform-managed authorization rosters.
type RosterRepository struct{ pool *pgxpool.Pool }

func NewRosterRepository(pool *pgxpool.Pool) *RosterRepository { return &RosterRepository{pool: pool} }

func (r *RosterRepository) ListApprovers(ctx context.Context) ([]*models.RosterMember, error) {
	members, err := r.list(ctx, `SELECT tm.user_id, u.lfid, tm.created_on, tm.updated_on FROM mentorship_approver_team_members tm JOIN users u ON u.id = tm.user_id ORDER BY u.lfid`, "")
	for _, member := range members {
		member.ObjectUID = "global"
	}
	return members, err
}

func (r *RosterRepository) AddApprover(ctx context.Context, userID string) (*models.RosterMember, error) {
	return r.add(ctx, userID)
}

func (r *RosterRepository) RemoveApprover(ctx context.Context, userID string) error {
	return r.remove(ctx, `DELETE FROM mentorship_approver_team_members WHERE user_id = $1`, userID)
}

func (r *RosterRepository) list(ctx context.Context, query, uid string) ([]*models.RosterMember, error) {
	var rows pgx.Rows
	var err error
	if uid == "" {
		rows, err = r.pool.Query(ctx, query)
	} else {
		rows, err = r.pool.Query(ctx, query, uid)
	}
	if err != nil {
		return nil, fmt.Errorf("list roster: %w", err)
	}
	defer rows.Close()
	members := []*models.RosterMember{}
	for rows.Next() {
		m := &models.RosterMember{ObjectUID: uid}
		if err := rows.Scan(&m.UserID, &m.LFID, &m.CreatedOn, &m.UpdatedOn); err != nil {
			return nil, fmt.Errorf("scan roster: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *RosterRepository) add(ctx context.Context, userID string) (*models.RosterMember, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin roster transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO mentorship_approver_team_members (user_id) VALUES ($1) ON CONFLICT (user_id) DO UPDATE SET updated_on = NOW()`, userID)
	if err != nil {
		return nil, fmt.Errorf("add roster member: %w", err)
	}
	var m models.RosterMember
	if err := tx.QueryRow(ctx, `SELECT id, lfid FROM users WHERE id = $1`, userID).Scan(&m.UserID, &m.LFID); err != nil {
		return nil, domain.ErrUserNotFound
	}
	m.ObjectUID = "global"
	if _, err := tx.Exec(ctx, `INSERT INTO fga_outbox (marker_kind, object_type, object_uid, relation, username, desired_operation) VALUES ('membership', 'mentorship_approver_team', 'global', 'member', $1, 'sync') ON CONFLICT (object_type, object_uid, relation, username) WHERE marker_kind = 'membership' DO UPDATE SET desired_operation = 'sync', generation = fga_outbox.generation + 1, state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END, claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END, claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END, attempts = 0, next_attempt_at = NOW(), last_error = NULL, updated_on = NOW()`, m.LFID); err != nil {
		return nil, fmt.Errorf("enqueue roster marker: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit roster transaction: %w", err)
	}
	return &m, nil
}

func (r *RosterRepository) remove(ctx context.Context, query, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("remove roster member: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	var lfid string
	if err := tx.QueryRow(ctx, `SELECT lfid FROM users WHERE id = $1`, userID).Scan(&lfid); err != nil {
		return fmt.Errorf("resolve roster LFID: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO fga_outbox (marker_kind, object_type, object_uid, relation, username, desired_operation) VALUES ('membership', 'mentorship_approver_team', 'global', 'member', $1, 'remove') ON CONFLICT (object_type, object_uid, relation, username) WHERE marker_kind = 'membership' DO UPDATE SET desired_operation = 'remove', generation = fga_outbox.generation + 1, state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END, claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END, claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END, attempts = 0, next_attempt_at = NOW(), last_error = NULL, updated_on = NOW()`, lfid); err != nil {
		return fmt.Errorf("enqueue roster removal marker: %w", err)
	}
	return tx.Commit(ctx)
}
