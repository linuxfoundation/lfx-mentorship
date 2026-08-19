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

var enrollmentTracer = otel.Tracer("enrollments-db")

// EnrollmentRepository implements domain.EnrollmentRepository against PostgreSQL.
type EnrollmentRepository struct {
	pool *pgxpool.Pool
}

// NewEnrollmentRepository creates a new EnrollmentRepository.
func NewEnrollmentRepository(pool *pgxpool.Pool) *EnrollmentRepository {
	return &EnrollmentRepository{pool: pool}
}

const enrollmentCols = `
	id, program_term_id, mentee_user_id, status, program_term_status,
	start_date_time, end_date_time, tasks_submitted, admin_notified,
	created_on, updated_on`

func scanEnrollment(row pgx.Row) (*models.Enrollment, error) {
	var e models.Enrollment
	err := row.Scan(
		&e.ID, &e.ProgramTermID, &e.MenteeUserID, &e.Status, &e.ProgramTermStatus,
		&e.StartDateTime, &e.EndDateTime, &e.TasksSubmitted, &e.AdminNotified,
		&e.CreatedOn, &e.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetByID returns the enrollment with the given UUID or ErrEnrollmentNotFound.
func (r *EnrollmentRepository) GetByID(ctx context.Context, id string) (*models.Enrollment, error) {
	ctx, span := enrollmentTracer.Start(ctx, "db.enrollments.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("db.enrollment_id", id))

	q := `SELECT ` + enrollmentCols + ` FROM enrollments WHERE id = $1`
	e, err := scanEnrollment(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrEnrollmentNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get enrollment by id: %w", err)
	}
	return e, nil
}

// ListByProgramTerm returns paginated enrollments for a program term.
func (r *EnrollmentRepository) ListByProgramTerm(ctx context.Context, programTermID string, filter models.EnrollmentFilter) ([]*models.Enrollment, *models.PaginationMeta, error) {
	ctx, span := enrollmentTracer.Start(ctx, "db.enrollments.ListByProgramTerm")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_term_id", programTermID))

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{programTermID}
	where := ` WHERE program_term_id = $1`
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM enrollments`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count enrollments: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT ` + enrollmentCols + ` FROM enrollments` + where +
		fmt.Sprintf(` ORDER BY created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list enrollments: %w", err)
	}
	defer rows.Close()

	var enrollments []*models.Enrollment
	for rows.Next() {
		e, err := scanEnrollment(rows)
		if err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan enrollment: %w", err)
		}
		enrollments = append(enrollments, e)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	if enrollments == nil {
		enrollments = []*models.Enrollment{}
	}
	return enrollments, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// Create inserts a new enrollment and returns the persisted record.
func (r *EnrollmentRepository) Create(ctx context.Context, programTermID string, input models.EnrollmentCreateInput) (*models.Enrollment, error) {
	ctx, span := enrollmentTracer.Start(ctx, "db.enrollments.Create")
	defer span.End()

	const q = `
		INSERT INTO enrollments (id, program_term_id, mentee_user_id, status, program_term_status, start_date_time, end_date_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + enrollmentCols

	e, err := scanEnrollment(r.pool.QueryRow(ctx, q,
		input.ID, programTermID, input.MenteeUserID, input.Status,
		input.ProgramTermStatus, input.StartDateTime, input.EndDateTime,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create enrollment: %w", err)
	}
	return e, nil
}

// Update patches an enrollment's status, dates, and flags.
func (r *EnrollmentRepository) Update(ctx context.Context, id string, input models.EnrollmentUpdateInput) (*models.Enrollment, error) {
	ctx, span := enrollmentTracer.Start(ctx, "db.enrollments.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.enrollment_id", id))

	const q = `
		UPDATE enrollments SET
			status              = COALESCE($2, status),
			program_term_status = COALESCE($3, program_term_status),
			start_date_time     = COALESCE($4, start_date_time),
			end_date_time       = COALESCE($5, end_date_time),
			tasks_submitted     = COALESCE($6, tasks_submitted),
			admin_notified      = COALESCE($7, admin_notified)
		WHERE id = $1
		RETURNING ` + enrollmentCols

	e, err := scanEnrollment(r.pool.QueryRow(ctx, q,
		id, input.Status, input.ProgramTermStatus,
		input.StartDateTime, input.EndDateTime,
		input.TasksSubmitted, input.AdminNotified,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrEnrollmentNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update enrollment: %w", err)
	}
	return e, nil
}
