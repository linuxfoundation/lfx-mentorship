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
	id, enrollment_id, program_term_id, assignee_id, owner_id,
	name, description, category, status, application_status, program_term_status,
	custom, submit_file, file, due_date, created_by,
	created_on, updated_on`

func scanTask(row pgx.Row) (*models.Task, error) {
	var t models.Task
	err := row.Scan(
		&t.ID, &t.EnrollmentID, &t.ProgramTermID, &t.AssigneeID, &t.OwnerID,
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

// ListByEnrollment returns paginated tasks for an enrollment.
func (r *TaskRepository) ListByEnrollment(ctx context.Context, enrollmentID string, filter models.TaskFilter) ([]*models.Task, *models.PaginationMeta, error) {
	ctx, span := taskTracer.Start(ctx, "db.tasks.ListByEnrollment")
	defer span.End()
	span.SetAttributes(attribute.String("db.enrollment_id", enrollmentID))

	return r.listWithFilter(ctx, span, ` WHERE enrollment_id = $1`, enrollmentID, filter)
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

// Create inserts a new task linked to the given enrollment.
func (r *TaskRepository) Create(ctx context.Context, enrollmentID string, input models.TaskCreateInput) (*models.Task, error) {
	ctx, span := taskTracer.Start(ctx, "db.tasks.Create")
	defer span.End()

	const q = `
		INSERT INTO tasks (
			id, enrollment_id, program_term_id, assignee_id, owner_id,
			name, description, category, status, custom,
			submit_file, due_date, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING ` + taskCols

	t, err := scanTask(r.pool.QueryRow(ctx, q,
		input.ID, enrollmentID, input.ProgramTermID, input.AssigneeID, input.OwnerID,
		input.Name, input.Description, input.Category, input.Status, input.Custom,
		input.SubmitFile, input.DueDate, input.CreatedBy,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

// Update patches a task's mutable fields.
func (r *TaskRepository) Update(ctx context.Context, id string, input models.TaskUpdateInput) (*models.Task, error) {
	ctx, span := taskTracer.Start(ctx, "db.tasks.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.task_id", id))

	const q = `
		UPDATE tasks SET
			name               = COALESCE($2,  name),
			description        = COALESCE($3,  description),
			category           = COALESCE($4,  category),
			status             = COALESCE($5,  status),
			application_status = COALESCE($6,  application_status),
			program_term_status= COALESCE($7,  program_term_status),
			custom             = COALESCE($8,  custom),
			submit_file        = COALESCE($9,  submit_file),
			file               = COALESCE($10, file),
			due_date           = COALESCE($11, due_date)
		WHERE id = $1
		RETURNING ` + taskCols

	t, err := scanTask(r.pool.QueryRow(ctx, q,
		id, input.Name, input.Description, input.Category, input.Status,
		input.ApplicationStatus, input.ProgramTermStatus, input.Custom,
		input.SubmitFile, input.File, input.DueDate,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update task: %w", err)
	}
	return t, nil
}

// Delete removes a task.
func (r *TaskRepository) Delete(ctx context.Context, id string) error {
	ctx, span := taskTracer.Start(ctx, "db.tasks.Delete")
	defer span.End()

	cmd, err := r.pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete task: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}
