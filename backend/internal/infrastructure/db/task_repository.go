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
	"go.opentelemetry.io/otel/trace"
)

var taskTracer = otel.Tracer("tasks-db")

// TaskRepository implements domain.TaskRepository against PostgreSQL.
type TaskRepository struct {
	pool *pgxpool.Pool
}

// NewTaskRepository creates a new TaskRepository.
func NewTaskRepository(pool *pgxpool.Pool) *TaskRepository {
	return &TaskRepository{pool: pool}
}

const taskCols = `
	id, application_id, program_term_id, assignee_id, owner_id,
	name, description, category, status, application_status, program_term_status,
	custom, submit_file, file, due_date, created_by,
	created_on, updated_on`

func scanTask(row pgx.Row) (*models.Task, error) {
	var t models.Task
	err := row.Scan(
		&t.ID, &t.ApplicationID, &t.ProgramTermID, &t.AssigneeID, &t.OwnerID,
		&t.Name, &t.Description, &t.Category, &t.Status, &t.ApplicationStatus, &t.ProgramTermStatus,
		&t.Custom, &t.SubmitFile, &t.File, &t.DueDate, &t.CreatedBy,
		&t.CreatedOn, &t.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetByID returns the task with the given UUID or ErrTaskNotFound.
func (r *TaskRepository) GetByID(ctx context.Context, id string) (*models.Task, error) {
	ctx, span := taskTracer.Start(ctx, "db.tasks.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("db.task_id", id))

	q := `SELECT ` + taskCols + ` FROM tasks WHERE id = $1`
	t, err := scanTask(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get task by id: %w", err)
	}
	return t, nil
}

// ListByApplication returns paginated tasks for an application.
func (r *TaskRepository) ListByApplication(ctx context.Context, applicationID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	ctx, span := taskTracer.Start(ctx, "db.tasks.ListByApplication")
	defer span.End()
	span.SetAttributes(attribute.String("db.application_id", applicationID))

	return r.listWithFilter(ctx, span, ` WHERE application_id = $1`, applicationID, filter)
}

// ListByProgramTerm returns paginated tasks for a program term.
func (r *TaskRepository) ListByProgramTerm(ctx context.Context, programTermID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	ctx, span := taskTracer.Start(ctx, "db.tasks.ListByProgramTerm")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_term_id", programTermID))

	return r.listWithFilter(ctx, span, ` WHERE program_term_id = $1`, programTermID, filter)
}

func (r *TaskRepository) listWithFilter(ctx context.Context, span trace.Span, baseWhere, baseArg string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{baseArg}
	where := baseWhere
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.AssigneeID != "" {
		args = append(args, filter.AssigneeID)
		where += fmt.Sprintf(` AND assignee_id = $%d`, len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count tasks: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT ` + taskCols + ` FROM tasks` + where +
		fmt.Sprintf(` ORDER BY due_date ASC NULLS LAST, created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*models.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	if tasks == nil {
		tasks = []*models.Task{}
	}
	return tasks, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// Create inserts a new task linked to the given application.
func (r *TaskRepository) Create(ctx context.Context, applicationID string, input models.TaskCreateInput) (*models.Task, error) {
	ctx, span := taskTracer.Start(ctx, "db.tasks.Create")
	defer span.End()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create task transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Share-lock the parent so a concurrent application update re-syncs this task after commit.
	var applicationStatus models.ApplicationStatus
	var termStatus models.ProgramTermStatus
	err = tx.QueryRow(ctx, `
		SELECT a.status, pt.status
		FROM applications a JOIN program_terms pt ON pt.id = a.program_term_id
		WHERE a.id = $1 FOR SHARE OF a`, applicationID).Scan(&applicationStatus, &termStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrApplicationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolve task parent application: %w", err)
	}

	const q = `
		INSERT INTO tasks (
			id, application_id, program_term_id, assignee_id, owner_id,
			name, description, category, status, application_status, program_term_status, custom,
			submit_file, due_date, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING ` + taskCols

	t, err := scanTask(tx.QueryRow(ctx, q,
		input.ID, applicationID, input.ProgramTermID, input.AssigneeID, input.OwnerID,
		input.Name, input.Description, input.Category, input.Status, applicationStatus, termStatus, input.Custom,
		input.SubmitFile, input.DueDate, input.CreatedBy,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create task: %w", err)
	}
	if err := enqueueTaskMarker(ctx, tx, t, "update_access"); err != nil {
		return nil, err
	}
	if err := enqueueTaskIndex(ctx, tx, t, "created"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create task transaction: %w", err)
	}
	return t, nil
}

// Update patches a task's mutable fields. A nil field is left unchanged; an empty submit_file
// or due_date clears it.
func (r *TaskRepository) Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error) {
	ctx, span := taskTracer.Start(ctx, "db.tasks.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.task_id", id))
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update task transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockTaskApplication(ctx, tx, id); err != nil {
		return nil, err
	}

	const q = `
		UPDATE tasks SET
			name               = COALESCE($2,  name),
			description        = COALESCE($3,  description),
			category           = COALESCE($4,  category),
			status             = COALESCE($5,  status),
			application_status = COALESCE($6,  application_status),
			program_term_status= COALESCE($7,  program_term_status),
			custom             = COALESCE($8,  custom),
			submit_file        = CASE WHEN $9 = '' THEN NULL ELSE COALESCE($9,  submit_file) END,
			file               = COALESCE($10, file),
			due_date           = CASE WHEN $11 = '' THEN NULL ELSE COALESCE($11, due_date) END
		WHERE id = $1
		  -- A task that requires a file cannot be submitted without one, whether it is being
		  -- submitted or its file requirement is being turned on.
		  AND NOT (COALESCE($5, CASE WHEN $9::text IS NOT NULL THEN status END, '') = $12
		           AND COALESCE($9, submit_file, '') <> ''
		           AND COALESCE($10, file, '') = '')
		RETURNING ` + taskCols

	t, err := scanTask(tx.QueryRow(ctx, q,
		id, input.Name, input.Description, input.Category, input.Status,
		input.ApplicationStatus, input.ProgramTermStatus, input.Custom,
		input.SubmitFile, input.File, input.DueDate, models.TaskStatusSubmitted,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, missOrRequiredFile(ctx, tx, id)
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update task: %w", err)
	}
	if err := enqueueTaskMarker(ctx, tx, t, "update_access"); err != nil {
		return nil, err
	}
	if err := enqueueTaskIndex(ctx, tx, t, "updated"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update task transaction: %w", err)
	}
	return t, nil
}

// Delete removes a task.
func (r *TaskRepository) Delete(ctx context.Context, id string) error {
	ctx, span := taskTracer.Start(ctx, "db.tasks.Delete")
	defer span.End()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete task transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskCols+` FROM tasks WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrTaskNotFound
	}
	if err != nil {
		return fmt.Errorf("load task before delete: %w", err)
	}

	cmd, err := tx.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete task: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrTaskNotFound
	}
	if err := queueObjectDeletions(ctx, tx, domain.ObjectBucketAttachments, current.File); err != nil {
		return err
	}
	if err := enqueueTaskMarker(ctx, tx, current, "delete_access"); err != nil {
		return err
	}
	if err := enqueueIndexDelete(ctx, tx, "mentorship_task", current.ID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete task transaction: %w", err)
	}
	return nil
}

// errWithdrawnTaskLocked rejects a write to a task whose application is withdrawn. Withdrawn is
// terminal and the application is kept as history when its applicant reapplies, so its tasks
// stay as they were at withdrawal.
var errWithdrawnTaskLocked = fmt.Errorf("%w: a withdrawn application's tasks cannot change", domain.ErrStateLocked)

// lockTaskApplication share-locks the application a task belongs to and returns
// errWithdrawnTaskLocked when it is withdrawn. It reads the application row rather than the
// task's denormalised application_status, and withdrawing updates that row, so a withdrawal
// either commits first and is seen here or waits until this task write commits.
func lockTaskApplication(ctx context.Context, tx pgx.Tx, taskID string) error {
	var status models.ApplicationStatus
	err := tx.QueryRow(ctx, `
		SELECT a.status FROM tasks t JOIN applications a ON a.id = t.application_id
		WHERE t.id = $1 FOR SHARE OF a`, taskID).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// A missing task, or one with no application, is left to the write itself.
		return nil
	case err != nil:
		return fmt.Errorf("lock task application: %w", err)
	case status == models.ApplicationStatusWithdrawn:
		return errWithdrawnTaskLocked
	}
	return nil
}

// missOrRequiredFile resolves an update that matched no row into not-found or a submitted task
// left without its required file.
func missOrRequiredFile(ctx context.Context, tx pgx.Tx, id string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tasks WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("check task after missed update: %w", err)
	}
	if !exists {
		return domain.ErrTaskNotFound
	}
	return fmt.Errorf("%w: a submitted task that requires a file must have one uploaded", domain.ErrInvalidInput)
}

func enqueueTaskMarker(ctx context.Context, tx pgx.Tx, task *models.Task, operation string) error {
	const q = `
		INSERT INTO fga_outbox (marker_kind, object_type, object_uid, desired_operation)
		VALUES ('object', 'mentorship_task', $1, $2)
		ON CONFLICT (object_type, object_uid) WHERE marker_kind = 'object'
		DO UPDATE SET desired_operation = EXCLUDED.desired_operation,
		              generation = fga_outbox.generation + 1,
		              state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END, claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END, claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
		              attempts = 0, next_attempt_at = NOW(), last_error = NULL,
		              updated_on = NOW()`
	if operation == "delete_access" {
		if _, err := tx.Exec(ctx, q, task.ID, operation); err != nil {
			return fmt.Errorf("enqueue task FGA marker: %w", err)
		}
		return nil
	}
	if task.ApplicationID == nil || *task.ApplicationID == "" {
		operation = "delete_access"
		if _, err := tx.Exec(ctx, q, task.ID, operation); err != nil {
			return fmt.Errorf("enqueue task FGA marker: %w", err)
		}
		return nil
	}
	var lfid string
	const lookup = `
		SELECT u.lfid
		FROM applications a
		JOIN users u ON u.id = $2
		WHERE a.id = $1`
	if err := tx.QueryRow(ctx, lookup, *task.ApplicationID, task.AssigneeID).Scan(&lfid); err != nil {
		return fmt.Errorf("resolve task FGA parent and assignee: %w", err)
	}
	if lfid == "" {
		return fmt.Errorf("task assignee %s has no LFID", task.AssigneeID)
	}
	if _, err := tx.Exec(ctx, q, task.ID, operation); err != nil {
		return fmt.Errorf("enqueue task FGA marker: %w", err)
	}
	return nil
}
