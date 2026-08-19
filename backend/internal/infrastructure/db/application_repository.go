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

var applicationTracer = otel.Tracer("applications-db")

// ApplicationRepository implements domain.ApplicationRepository against PostgreSQL.
type ApplicationRepository struct {
	pool *pgxpool.Pool
}

// NewApplicationRepository creates a new ApplicationRepository.
func NewApplicationRepository(pool *pgxpool.Pool) *ApplicationRepository {
	return &ApplicationRepository{pool: pool}
}

const applicationCols = `
	id, program_term_id, user_id, role, status, program_term_status,
	tasks_submitted, admin_notified, created_on, updated_on`

func scanApplication(row pgx.Row) (*models.Application, error) {
	var a models.Application
	err := row.Scan(
		&a.ID, &a.ProgramTermID, &a.UserID, &a.Role, &a.Status, &a.ProgramTermStatus,
		&a.TasksSubmitted, &a.AdminNotified, &a.CreatedOn, &a.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetByID returns the application with the given UUID or ErrApplicationNotFound.
func (r *ApplicationRepository) GetByID(ctx context.Context, id string) (*models.Application, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("db.application_id", id))

	q := `SELECT ` + applicationCols + ` FROM applications WHERE id = $1`
	a, err := scanApplication(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrApplicationNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get application by id: %w", err)
	}
	return a, nil
}

// ListByProgramTerm returns paginated applications for a program term.
func (r *ApplicationRepository) ListByProgramTerm(ctx context.Context, programTermID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.ListByProgramTerm")
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
	if filter.Role != "" {
		args = append(args, filter.Role)
		where += fmt.Sprintf(` AND role = $%d`, len(args))
	}
	if filter.UserID != "" {
		args = append(args, filter.UserID)
		where += fmt.Sprintf(` AND user_id = $%d`, len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM applications`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count applications: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT ` + applicationCols + ` FROM applications` + where +
		fmt.Sprintf(` ORDER BY created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()

	var apps []*models.Application
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan application: %w", err)
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	if apps == nil {
		apps = []*models.Application{}
	}
	return apps, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// ListByUser returns paginated applications for a specific user.
func (r *ApplicationRepository) ListByUser(ctx context.Context, userID string, filter models.ApplicationFilter) ([]*models.Application, *models.PaginationMeta, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.ListByUser")
	defer span.End()
	span.SetAttributes(attribute.String("db.user_id", userID))

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{userID}
	where := ` WHERE user_id = $1`
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.Role != "" {
		args = append(args, filter.Role)
		where += fmt.Sprintf(` AND role = $%d`, len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM applications`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count user applications: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT ` + applicationCols + ` FROM applications` + where +
		fmt.Sprintf(` ORDER BY created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list user applications: %w", err)
	}
	defer rows.Close()

	var apps []*models.Application
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan application: %w", err)
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	if apps == nil {
		apps = []*models.Application{}
	}
	return apps, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// Create inserts a new application and returns the persisted record.
func (r *ApplicationRepository) Create(ctx context.Context, programTermID string, input models.ApplicationCreateInput) (*models.Application, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.Create")
	defer span.End()

	const q = `
		INSERT INTO applications (id, program_term_id, user_id, role, status, program_term_status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + applicationCols

	a, err := scanApplication(r.pool.QueryRow(ctx, q,
		input.ID, programTermID, input.UserID, input.Role, input.Status, input.ProgramTermStatus,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create application: %w", err)
	}
	return a, nil
}

// Update patches an application's status fields.
func (r *ApplicationRepository) Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.application_id", id))

	const q = `
		UPDATE applications SET
			status             = COALESCE($2, status),
			program_term_status= COALESCE($3, program_term_status),
			tasks_submitted    = COALESCE($4, tasks_submitted),
			admin_notified     = COALESCE($5, admin_notified)
		WHERE id = $1
		RETURNING ` + applicationCols

	a, err := scanApplication(r.pool.QueryRow(ctx, q,
		id, input.Status, input.ProgramTermStatus, input.TasksSubmitted, input.AdminNotified,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrApplicationNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update application: %w", err)
	}
	return a, nil
}

// Delete removes an application.
func (r *ApplicationRepository) Delete(ctx context.Context, id string) error {
	ctx, span := applicationTracer.Start(ctx, "db.applications.Delete")
	defer span.End()

	cmd, err := r.pool.Exec(ctx, `DELETE FROM applications WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete application: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrApplicationNotFound
	}
	return nil
}
