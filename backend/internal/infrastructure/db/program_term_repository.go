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

var programTermTracer = otel.Tracer("program-terms-db")

// ProgramTermRepository implements domain.ProgramTermRepository against PostgreSQL.
type ProgramTermRepository struct {
	pool *pgxpool.Pool
}

// NewProgramTermRepository creates a new ProgramTermRepository.
func NewProgramTermRepository(pool *pgxpool.Pool) *ProgramTermRepository {
	return &ProgramTermRepository{pool: pool}
}

const programTermCols = `
	id, program_id, name, status, active_users,
	start_date_time, end_date_time, application_start_date, application_end_date,
	created_on, updated_on`

func scanProgramTerm(row pgx.Row) (*models.ProgramTerm, error) {
	var t models.ProgramTerm
	err := row.Scan(
		&t.ID, &t.ProgramID, &t.Name, &t.Status, &t.ActiveUsers,
		&t.StartDateTime, &t.EndDateTime, &t.ApplicationStartDate, &t.ApplicationEndDate,
		&t.CreatedOn, &t.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetByID returns the program term with the given UUID or ErrProgramTermNotFound.
func (r *ProgramTermRepository) GetByID(ctx context.Context, id string) (*models.ProgramTerm, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("db.term_id", id))

	q := `SELECT` + programTermCols + ` FROM program_terms WHERE id = $1`
	t, err := scanProgramTerm(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramTermNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program term by id: %w", err)
	}
	return t, nil
}

// GetByProgramAndID returns the term for the given program + id, or ErrProgramTermNotFound when the term does not belong to the parent program.
func (r *ProgramTermRepository) GetByProgramAndID(ctx context.Context, programID, id string) (*models.ProgramTerm, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.GetByProgramAndID")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", programID), attribute.String("db.term_id", id))

	q := `SELECT` + programTermCols + ` FROM program_terms WHERE program_id = $1 AND id = $2`
	t, err := scanProgramTerm(r.pool.QueryRow(ctx, q, programID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramTermNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program term by program and id: %w", err)
	}
	return t, nil
}

// ListByProgram returns all terms for a program, paginated and optionally filtered by status.
func (r *ProgramTermRepository) ListByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTerm, *models.PaginationMeta, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.ListByProgram")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", programID))

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{programID}
	where := ` WHERE program_id = $1`
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM program_terms`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count program terms: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT` + programTermCols + ` FROM program_terms` + where +
		fmt.Sprintf(` ORDER BY start_date_time DESC NULLS LAST LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list program terms: %w", err)
	}
	defer rows.Close()

	var terms []*models.ProgramTerm
	for rows.Next() {
		t, err := scanProgramTerm(rows)
		if err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan program term: %w", err)
		}
		terms = append(terms, t)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	if terms == nil {
		terms = []*models.ProgramTerm{}
	}
	return terms, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// Create inserts a new program term and returns the persisted record.
func (r *ProgramTermRepository) Create(ctx context.Context, input models.ProgramTermCreateInput) (*models.ProgramTerm, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.Create")
	defer span.End()

	const q = `
		INSERT INTO program_terms (
			id, program_id, name, status, active_users,
			start_date_time, end_date_time, application_start_date, application_end_date
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING` + programTermCols

	t, err := scanProgramTerm(r.pool.QueryRow(ctx, q,
		input.ID, input.ProgramID, input.Name, input.Status, input.ActiveUsers,
		input.StartDateTime, input.EndDateTime, input.ApplicationStartDate, input.ApplicationEndDate,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program term: %w", err)
	}
	return t, nil
}

// Update patches program term fields and returns the updated record.
func (r *ProgramTermRepository) Update(ctx context.Context, id string, input models.ProgramTermUpdateInput) (*models.ProgramTerm, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.term_id", id))

	const q = `
		UPDATE program_terms SET
			name                   = COALESCE($2, name),
			status                 = COALESCE($3, status),
			active_users           = COALESCE($4, active_users),
			start_date_time        = COALESCE($5, start_date_time),
			end_date_time          = COALESCE($6, end_date_time),
			application_start_date = COALESCE($7, application_start_date),
			application_end_date   = COALESCE($8, application_end_date)
		WHERE id = $1
		RETURNING` + programTermCols

	t, err := scanProgramTerm(r.pool.QueryRow(ctx, q,
		id, input.Name, input.Status, input.ActiveUsers,
		input.StartDateTime, input.EndDateTime, input.ApplicationStartDate, input.ApplicationEndDate,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramTermNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program term: %w", err)
	}
	return t, nil
}

// Delete removes the program term with the given ID.
func (r *ProgramTermRepository) Delete(ctx context.Context, id string) error {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.Delete")
	defer span.End()

	cmd, err := r.pool.Exec(ctx, `UPDATE program_terms SET status = 'deleted', updated_on = NOW() WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program term: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrProgramTermNotFound
	}
	return nil
}

// CloseWithBulkDecline declines pending applications, emits their FGA updates,
// and closes the term in one transaction.
func (r *ProgramTermRepository) CloseWithBulkDecline(ctx context.Context, id string) (*models.ProgramTerm, int, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.CloseWithBulkDecline")
	defer span.End()
	span.SetAttributes(attribute.String("db.term_id", id))
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("begin close term transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	term, err := scanProgramTerm(tx.QueryRow(ctx, `SELECT`+programTermCols+` FROM program_terms WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, domain.ErrProgramTermNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("load term for close: %w", err)
	}
	if term.Status != models.ProgramTermStatusOpen {
		return nil, 0, fmt.Errorf("%w: only open terms can be closed", domain.ErrInvalidStateTransition)
	}

	var accepted int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM applications WHERE program_term_id = $1 AND status = 'accepted'`, id).Scan(&accepted); err != nil {
		return nil, 0, fmt.Errorf("count accepted applications for close: %w", err)
	}
	if accepted > 0 {
		return nil, 0, fmt.Errorf("%w: term has %d accepted application(s)", domain.ErrStateLocked, accepted)
	}

	rows, err := tx.Query(ctx, `UPDATE applications SET status = 'declined' WHERE program_term_id = $1 AND status = 'pending' RETURNING `+applicationCols, id)
	if err != nil {
		return nil, 0, fmt.Errorf("decline pending applications for close: %w", err)
	}
	applications := make([]*models.Application, 0)
	for rows.Next() {
		application, scanErr := scanApplication(rows)
		if scanErr != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("scan declined application for close: %w", scanErr)
		}
		applications = append(applications, application)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, fmt.Errorf("decline pending applications rows: %w", err)
	}
	rows.Close()
	for _, application := range applications {
		if err := enqueueApplicationMarker(ctx, tx, application, "update_access"); err != nil {
			return nil, 0, err
		}
	}

	closed := models.ProgramTermStatusClosed
	updated, err := scanProgramTerm(tx.QueryRow(ctx, `UPDATE program_terms SET status = $2 WHERE id = $1 RETURNING`+programTermCols, id, closed))
	if err != nil {
		return nil, 0, fmt.Errorf("close term: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, 0, fmt.Errorf("commit close term transaction: %w", err)
	}
	return updated, len(applications), nil
}

// CountOpenTermsByProgram returns the count of terms with status='open' for a program.
func (r *ProgramTermRepository) CountOpenTermsByProgram(ctx context.Context, programID string) (int, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.CountOpenTermsByProgram")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", programID))

	var count int
	// NOTE: status literal must stay in sync with models.ProgramTermStatus.
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM program_terms WHERE program_id = $1 AND status = 'open'`, programID).Scan(&count)
	if err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("count open terms: %w", err)
	}
	return count, nil
}
