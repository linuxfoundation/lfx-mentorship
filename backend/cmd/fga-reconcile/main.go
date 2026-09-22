// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Command fga-reconcile re-dirties current authorization state for relay delivery.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(context.Background(), logger); err != nil {
		logger.Error("fga reconciliation failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	pool, err := db.NewPool(ctx, db.PoolConfig{MaxConns: 10, MinConns: 2})
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()

	outbox := db.NewFGAOutboxRepository(pool)
	counts := map[string]int{}
	if err := validateAuthorizationData(ctx, pool); err != nil {
		return fmt.Errorf("authorization data audit: %w", err)
	}

	if err := reconcileObjects(ctx, pool, outbox, "programs", "mentorship_program", &counts); err != nil {
		return err
	}
	if err := reconcileObjects(ctx, pool, outbox, "applications", "mentorship_application", &counts); err != nil {
		return err
	}
	if err := reconcileObjects(ctx, pool, outbox, "tasks", "mentorship_task", &counts); err != nil {
		return err
	}
	if err := reconcileMemberships(ctx, pool, outbox, &counts); err != nil {
		return err
	}
	if err := reconcileMembershipTombstones(ctx, pool, outbox, &counts); err != nil {
		return err
	}
	if err := reconcileApprovers(ctx, pool, outbox, &counts); err != nil {
		return err
	}
	if err := reconcileProjectAdmins(ctx, pool, outbox, &counts); err != nil {
		return err
	}

	logger.Info("FGA reconciliation markers queued", "counts", counts)
	return nil
}

func validateAuthorizationData(ctx context.Context, pool Queryer) error {
	checks := []struct {
		name  string
		query string
	}{
		{
			name:  "programs missing project_uid",
			query: `SELECT COUNT(*) FROM programs WHERE project_uid IS NULL OR BTRIM(project_uid) = ''`,
		},
		{
			name:  "tasks missing application parent",
			query: `SELECT COUNT(*) FROM tasks WHERE application_id IS NULL`,
		},
		{
			name: "active program members missing LFID",
			query: `SELECT COUNT(*)
				FROM program_members pm
				JOIN users u ON u.id = pm.user_id
				WHERE pm.status = 'active' AND (u.lfid IS NULL OR BTRIM(u.lfid) = '')`,
		},
		{
			name: "approver team members missing LFID",
			query: `SELECT COUNT(*)
				FROM mentorship_approver_team_members tm
				JOIN users u ON u.id = tm.user_id
				WHERE u.lfid IS NULL OR BTRIM(u.lfid) = ''`,
		},
		{
			name: "project admins missing LFID",
			query: `SELECT COUNT(*)
				FROM mentorship_program_admins pa
				JOIN users u ON u.id = pa.user_id
				WHERE u.lfid IS NULL OR BTRIM(u.lfid) = ''`,
		},
		{
			name: "applications missing program term",
			query: `SELECT COUNT(*)
				FROM applications a
				LEFT JOIN program_terms pt ON pt.id = a.program_term_id
				WHERE pt.id IS NULL`,
		},
		{
			name: "tasks missing application record",
			query: `SELECT COUNT(*)
				FROM tasks t
				LEFT JOIN applications a ON a.id = t.application_id
				WHERE a.id IS NULL`,
		},
		{
			name:  "application owners missing LFID",
			query: `SELECT COUNT(*) FROM applications a JOIN users u ON u.id = a.user_id WHERE u.lfid IS NULL OR BTRIM(u.lfid) = ''`,
		},
		{
			name:  "task assignees missing LFID",
			query: `SELECT COUNT(*) FROM tasks t JOIN users u ON u.id = t.assignee_id WHERE u.lfid IS NULL OR BTRIM(u.lfid) = ''`,
		},
	}

	var failures []string
	for _, check := range checks {
		rows, err := pool.Query(ctx, check.query)
		if err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
		if !rows.Next() {
			rows.Close()
			return fmt.Errorf("%s: query returned no result", check.name)
		}
		var count int
		if err := rows.Scan(&count); err != nil {
			rows.Close()
			return fmt.Errorf("%s: scan count: %w", check.name, err)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("%s: rows: %w", check.name, err)
		}
		if count > 0 {
			failures = append(failures, fmt.Sprintf("%s: %d", check.name, count))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("repair or quarantine before seeding: %s", strings.Join(failures, "; "))
	}
	return nil
}

func reconcileObjects(ctx context.Context, pool Queryer, outbox *db.FGAOutboxRepository, table, objectType string, counts *map[string]int) error {
	rows, err := pool.Query(ctx, "SELECT id FROM "+table)
	if err != nil {
		return fmt.Errorf("list %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan %s: %w", table, err)
		}
		if err := outbox.ReconcileObject(ctx, objectType, id, "update_access"); err != nil {
			return err
		}
		(*counts)[objectType]++
	}
	return rows.Err()
}

func reconcileMemberships(ctx context.Context, pool Queryer, outbox *db.FGAOutboxRepository, counts *map[string]int) error {
	rows, err := pool.Query(ctx, `
		SELECT pm.program_id, pm.member_type, u.lfid
		FROM program_members pm
		JOIN users u ON u.id = pm.user_id
		WHERE pm.status = 'active' AND u.lfid IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("list program memberships: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var programID, memberType, lfid string
		if err := rows.Scan(&programID, &memberType, &lfid); err != nil {
			return fmt.Errorf("scan program membership: %w", err)
		}
		relation := "mentor"
		if memberType == "program_admin" {
			relation = "writer"
		}
		if err := outbox.ReconcileMembership(ctx, "mentorship_program", programID, relation, lfid); err != nil {
			return err
		}
		(*counts)["mentorship_program_membership"]++
	}
	return rows.Err()
}

func reconcileApprovers(ctx context.Context, pool Queryer, outbox *db.FGAOutboxRepository, counts *map[string]int) error {
	rows, err := pool.Query(ctx, `
		SELECT u.lfid
		FROM mentorship_approver_team_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE u.lfid IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("list approver team members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var lfid string
		if err := rows.Scan(&lfid); err != nil {
			return fmt.Errorf("scan approver team member: %w", err)
		}
		if err := outbox.ReconcileMembership(ctx, "mentorship_approver_team", "global", "member", lfid); err != nil {
			return err
		}
		(*counts)["mentorship_approver_team"]++
	}
	return rows.Err()
}

func reconcileProjectAdmins(ctx context.Context, pool Queryer, outbox *db.FGAOutboxRepository, counts *map[string]int) error {
	rows, err := pool.Query(ctx, `
		SELECT pa.project_uid, u.lfid
		FROM mentorship_program_admins pa
		JOIN users u ON u.id = pa.user_id
		WHERE u.lfid IS NOT NULL AND BTRIM(u.lfid) <> ''`)
	if err != nil {
		return fmt.Errorf("list project admins: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var projectUID, lfid string
		if err := rows.Scan(&projectUID, &lfid); err != nil {
			return fmt.Errorf("scan project admin: %w", err)
		}
		if err := outbox.ReconcileMembership(ctx, "project", projectUID, "mentorship_program_admin", lfid); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `DELETE FROM fga_membership_tombstones WHERE object_type = 'project' AND object_uid = $1 AND relation = 'mentorship_program_admin' AND username = $2`, projectUID, lfid); err != nil {
			return fmt.Errorf("clear project admin tombstone: %w", err)
		}
		(*counts)["project_membership"]++
	}
	return rows.Err()
}

func reconcileMembershipTombstones(ctx context.Context, pool Queryer, outbox *db.FGAOutboxRepository, counts *map[string]int) error {
	rows, err := pool.Query(ctx, `
		SELECT object_type, object_uid, relation, username
		FROM fga_membership_tombstones
		WHERE last_reconciled_on IS NULL OR last_reconciled_on < NOW() - INTERVAL '5 minutes'
		ORDER BY deleted_on`)
	if err != nil {
		return fmt.Errorf("list membership tombstones: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var objectType, objectUID, relation, username string
		if err := rows.Scan(&objectType, &objectUID, &relation, &username); err != nil {
			return fmt.Errorf("scan membership tombstone: %w", err)
		}
		if err := outbox.ReconcileMembershipRemoval(ctx, objectType, objectUID, relation, username); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `
			UPDATE fga_membership_tombstones
			SET last_reconciled_on = NOW()
			WHERE object_type = $1 AND object_uid = $2 AND relation = $3 AND username = $4`,
			objectType, objectUID, relation, username); err != nil {
			return fmt.Errorf("mark membership tombstone reconciled: %w", err)
		}
		(*counts)["fga_membership_tombstone"]++
	}
	return rows.Err()
}

type Queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
