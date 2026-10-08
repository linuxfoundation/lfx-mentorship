// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var fileTracer = otel.Tracer("files-db")

// FileRepository implements domain.FileRepository against PostgreSQL.
type FileRepository struct {
	pool *pgxpool.Pool
}

// NewFileRepository creates a new FileRepository.
func NewFileRepository(pool *pgxpool.Pool) *FileRepository {
	return &FileRepository{pool: pool}
}

// ReplaceProgramLogo implements domain.FileRepository.
func (r *FileRepository) ReplaceProgramLogo(ctx context.Context, rep domain.FileReplacement) error {
	ctx, span := fileTracer.Start(ctx, "db.files.ReplaceProgramLogo")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", rep.RowID))

	return r.inTx(ctx, "replace program logo", func(tx pgx.Tx) error {
		cmd, err := tx.Exec(ctx, `
			UPDATE programs SET logo_url = $3
			WHERE id = $1 AND logo_url IS NOT DISTINCT FROM $2 AND status <> $4`, rep.RowID, rep.Previous, rep.Next, models.ProgramStatusArchived)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			return programLogoMiss(ctx, tx, rep.RowID)
		}
		if err := settleReplacement(ctx, tx, domain.ObjectBucketLogos, rep); err != nil {
			return err
		}
		return enqueueProgramIndexByID(ctx, tx, rep.RowID)
	})
}

// ReplaceTaskFile implements domain.FileRepository.
func (r *FileRepository) ReplaceTaskFile(ctx context.Context, rep domain.FileReplacement) error {
	ctx, span := fileTracer.Start(ctx, "db.files.ReplaceTaskFile")
	defer span.End()
	span.SetAttributes(attribute.String("db.task_id", rep.RowID))

	return r.inTx(ctx, "replace task file", func(tx pgx.Tx) error {
		if err := lockTaskApplication(ctx, tx, rep.RowID); err != nil {
			return err
		}
		t, err := scanTask(tx.QueryRow(ctx, `
			UPDATE tasks SET file = $3
			WHERE id = $1 AND file IS NOT DISTINCT FROM $2 AND status <> $4
			RETURNING `+taskCols, rep.RowID, rep.Previous, rep.Next, models.TaskStatusComplete))
		if errors.Is(err, pgx.ErrNoRows) {
			return taskFileMiss(ctx, tx, rep.RowID, func(s models.TaskStatus) bool { return s != models.TaskStatusComplete })
		}
		if err != nil {
			return err
		}
		if err := settleReplacement(ctx, tx, domain.ObjectBucketAttachments, rep); err != nil {
			return err
		}
		return enqueueTaskIndex(ctx, tx, t, "updated")
	})
}

// ClearProgramLogo implements domain.FileRepository.
func (r *FileRepository) ClearProgramLogo(ctx context.Context, programID, previous string) error {
	ctx, span := fileTracer.Start(ctx, "db.files.ClearProgramLogo")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", programID))

	return r.inTx(ctx, "clear program logo", func(tx pgx.Tx) error {
		cmd, err := tx.Exec(ctx, `
			UPDATE programs SET logo_url = NULL
			WHERE id = $1 AND logo_url = $2 AND status <> $3`, programID, previous, models.ProgramStatusArchived)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			return programLogoMiss(ctx, tx, programID)
		}
		if err := queueObjectDeletions(ctx, tx, domain.ObjectBucketLogos, &previous); err != nil {
			return err
		}
		return enqueueProgramIndexByID(ctx, tx, programID)
	})
}

// ClearTaskFile implements domain.FileRepository.
func (r *FileRepository) ClearTaskFile(ctx context.Context, taskID, previous string) error {
	ctx, span := fileTracer.Start(ctx, "db.files.ClearTaskFile")
	defer span.End()
	span.SetAttributes(attribute.String("db.task_id", taskID))

	removable := func(s models.TaskStatus) bool {
		return s == models.TaskStatusIncomplete || s == models.TaskStatusInProgress
	}
	return r.inTx(ctx, "clear task file", func(tx pgx.Tx) error {
		if err := lockTaskApplication(ctx, tx, taskID); err != nil {
			return err
		}
		t, err := scanTask(tx.QueryRow(ctx, `
			UPDATE tasks SET file = NULL
			WHERE id = $1 AND file = $2 AND status IN ($3, $4)
			RETURNING `+taskCols, taskID, previous, models.TaskStatusIncomplete, models.TaskStatusInProgress))
		if errors.Is(err, pgx.ErrNoRows) {
			return taskFileMiss(ctx, tx, taskID, removable)
		}
		if err != nil {
			return err
		}
		if err := queueObjectDeletions(ctx, tx, domain.ObjectBucketAttachments, &previous); err != nil {
			return err
		}
		return enqueueTaskIndex(ctx, tx, t, "updated")
	})
}

func (r *FileRepository) inTx(ctx context.Context, op string, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s transaction: %w", op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s transaction: %w", op, err)
	}
	return nil
}

// settleReplacement cancels the new locator's grace-period entry and queues the previous
// locator. A cancel that matches nothing means the relay already claimed the entry and is
// deleting the new object, so the replace must roll back rather than point at it.
func settleReplacement(ctx context.Context, tx pgx.Tx, bucket domain.ObjectBucket, rep domain.FileReplacement) error {
	cmd, err := tx.Exec(ctx, `DELETE FROM object_deletions WHERE id = $1 AND state = 'pending'`, rep.PendingDeletionID)
	if err != nil {
		return fmt.Errorf("cancel pending deletion of new file: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("%w: the upload expired before it was saved; retry it", domain.ErrConflict)
	}
	return queueObjectDeletions(ctx, tx, bucket, rep.Previous)
}

// programLogoMiss resolves a missed program logo write into not-found, an archived program, or a lost race.
func programLogoMiss(ctx context.Context, tx pgx.Tx, programID string) error {
	var status models.ProgramStatus
	err := tx.QueryRow(ctx, `SELECT status FROM programs WHERE id = $1`, programID).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.ErrProgramNotFound
	case err != nil:
		return fmt.Errorf("load program after missed logo write: %w", err)
	case status == models.ProgramStatusArchived:
		return fmt.Errorf("%w: an archived program's logo cannot change", domain.ErrStateLocked)
	default:
		return fmt.Errorf("%w: logo changed concurrently", domain.ErrConflict)
	}
}

// taskFileMiss resolves a missed task file write into not-found, a state rule, or a lost race.
func taskFileMiss(ctx context.Context, tx pgx.Tx, taskID string, allowed func(models.TaskStatus) bool) error {
	var status models.TaskStatus
	err := tx.QueryRow(ctx, `SELECT status FROM tasks WHERE id = $1`, taskID).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.ErrTaskNotFound
	case err != nil:
		return fmt.Errorf("load task after missed file write: %w", err)
	case !allowed(status):
		return fmt.Errorf("%w: a %s task's file cannot change", domain.ErrStateLocked, status)
	default:
		return fmt.Errorf("%w: task file changed concurrently", domain.ErrConflict)
	}
}
