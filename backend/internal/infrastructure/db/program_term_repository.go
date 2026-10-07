// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	where := ` WHERE program_id = $1 AND status <> 'deleted'`
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

func (r *ProgramTermRepository) ListManagementByProgram(ctx context.Context, programID string, filter models.ProgramTermFilter) ([]*models.ProgramTermManagementRow, *models.PaginationMeta, error) {
	limit, offset := filter.Limit, filter.Offset
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args := []any{programID}
	where := ` WHERE pt.program_id = $1 AND pt.status <> 'deleted'`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM program_terms pt`+where, args...).Scan(&total); err != nil {
		return nil, nil, err
	}
	args = append(args, limit, offset)
	q := `SELECT pt.id, pt.program_id, pt.name, pt.status, pt.active_users,
		pt.start_date_time, pt.end_date_time, pt.application_start_date, pt.application_end_date,
		pt.created_on, pt.updated_on,
		COUNT(a.id) FILTER (WHERE a.role = 'mentee' AND a.status = 'pending'), COUNT(a.id) FILTER (WHERE a.role = 'mentee' AND a.status = 'declined'),
		COUNT(a.id) FILTER (WHERE a.role = 'mentee' AND a.status = 'accepted'), COUNT(a.id) FILTER (WHERE a.role = 'mentee' AND a.status = 'graduated')
		FROM program_terms pt LEFT JOIN applications a ON a.program_term_id = pt.id` + where + ` GROUP BY pt.id ORDER BY pt.start_date_time DESC NULLS LAST` + fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	result := make([]*models.ProgramTermManagementRow, 0)
	for rows.Next() {
		var row models.ProgramTermManagementRow
		if err := rows.Scan(&row.ID, &row.ProgramID, &row.Name, &row.Status, &row.ActiveUsers, &row.StartDateTime, &row.EndDateTime, &row.ApplicationStartDate, &row.ApplicationEndDate, &row.CreatedOn, &row.UpdatedOn, &row.Pending, &row.Declined, &row.Accepted, &row.Graduated); err != nil {
			return nil, nil, err
		}
		result = append(result, &row)
	}
	return result, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, rows.Err()
}

// ListActiveByProgram returns every non-deleted term of a program, unpaginated.
func (r *ProgramTermRepository) ListActiveByProgram(ctx context.Context, programID string) ([]*models.ProgramTerm, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.ListActiveByProgram")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", programID))

	rows, err := r.pool.Query(ctx, `SELECT`+programTermCols+` FROM program_terms WHERE program_id = $1 AND status <> 'deleted' ORDER BY start_date_time DESC NULLS LAST`, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list active program terms: %w", err)
	}
	defer rows.Close()

	terms := []*models.ProgramTerm{}
	for rows.Next() {
		t, err := scanProgramTerm(rows)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan program term: %w", err)
		}
		terms = append(terms, t)
	}
	return terms, rows.Err()
}

// Create inserts a new program term and returns the persisted record.
func (r *ProgramTermRepository) Create(ctx context.Context, input models.ProgramTermCreateInput) (*models.ProgramTerm, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.Create")
	defer span.End()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create program term transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Every create takes the program lock, so it cannot race a term-set replacement.
	if input.Status == models.ProgramTermStatusOpen {
		err = lockProgramAndCheckOpenTerms(ctx, tx, input.ProgramID, "")
	} else {
		err = lockProgram(ctx, tx, input.ProgramID)
	}
	if err != nil {
		return nil, err
	}

	const q = `
		INSERT INTO program_terms (
			id, program_id, name, status, active_users,
			start_date_time, end_date_time, application_start_date, application_end_date
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING` + programTermCols

	t, err := scanProgramTerm(tx.QueryRow(ctx, q,
		input.ID, input.ProgramID, input.Name, input.Status, input.ActiveUsers,
		input.StartDateTime, input.EndDateTime, input.ApplicationStartDate, input.ApplicationEndDate,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program term: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create program term transaction: %w", err)
	}
	return t, nil
}

// Update patches program term fields and returns the updated record.
func (r *ProgramTermRepository) Update(ctx context.Context, id string, input models.ProgramTermUpdateInput) (*models.ProgramTerm, error) {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.term_id", id))

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update program term transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if input.Status != nil && *input.Status == models.ProgramTermStatusOpen {
		// Lock the program before the term (the UPDATE below), the order every
		// term-set write uses; a term's program_id never changes, so no lock is needed to read it.
		var programID string
		if err := tx.QueryRow(ctx, `SELECT program_id FROM program_terms WHERE id = $1`, id).Scan(&programID); errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProgramTermNotFound
		} else if err != nil {
			return nil, fmt.Errorf("load program for term update: %w", err)
		} else if err := lockProgramAndCheckOpenTerms(ctx, tx, programID, id); err != nil {
			return nil, err
		}
	}

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

	t, err := scanProgramTerm(tx.QueryRow(ctx, q,
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
	if input.Status != nil {
		if err := syncApplicationsWithTermState(ctx, tx, id, t.Status); err != nil {
			return nil, err
		}
		if err := enqueueProgramIndexByID(ctx, tx, t.ProgramID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update program term transaction: %w", err)
	}
	return t, nil
}

// lockProgram takes the program row lock that serializes writes to its term set.
func lockProgram(ctx context.Context, tx pgx.Tx, programID string) error {
	if err := tx.QueryRow(ctx, `SELECT id FROM programs WHERE id = $1 FOR UPDATE`, programID).Scan(new(string)); errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrProgramNotFound
	} else if err != nil {
		return fmt.Errorf("lock program for term write: %w", err)
	}
	return nil
}

func lockProgramAndCheckOpenTerms(ctx context.Context, tx pgx.Tx, programID, excludeTermID string) error {
	if err := lockProgram(ctx, tx, programID); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM program_terms WHERE program_id = $1 AND status = 'open' AND ($2 = '' OR id::text <> $2)`, programID, excludeTermID).Scan(&count); err != nil {
		return fmt.Errorf("count open terms for write: %w", err)
	}
	if count >= models.MaxOpenTermsPerProgram {
		return fmt.Errorf("%w: program already has %d open term(s) (max %d)", domain.ErrStateLocked, count, models.MaxOpenTermsPerProgram)
	}
	return nil
}

// lockProgramTerms locks and returns the program's non-deleted terms by ID. While
// held, no application can be added to these terms and none can change state.
func lockProgramTerms(ctx context.Context, tx pgx.Tx, programID string) (map[string]*models.ProgramTerm, error) {
	rows, err := tx.Query(ctx, `SELECT`+programTermCols+` FROM program_terms WHERE program_id = $1 AND status <> 'deleted' FOR UPDATE`, programID)
	if err != nil {
		return nil, fmt.Errorf("lock program terms: %w", err)
	}
	defer rows.Close()
	terms := map[string]*models.ProgramTerm{}
	for rows.Next() {
		t, err := scanProgramTerm(rows)
		if err != nil {
			return nil, fmt.Errorf("scan program term: %w", err)
		}
		terms[t.ID] = t
	}
	return terms, rows.Err()
}

// replaceProgramTerms makes terms the program's full term set. Entries with an ID
// update that term unless they match it, entries without one are created open,
// and unlisted terms are soft-deleted. It rejects an ID that is not a current
// term of the program, editing a historical term, removing a term that has
// applications, and adding terms beyond the open-term cap; a program already
// over the cap can still edit its terms.
//
// The caller must hold the program row lock. Every term-set write takes that
// lock before any term lock, so the term set read here is complete and stays
// so until commit.
func replaceProgramTerms(ctx context.Context, tx pgx.Tx, programID string, terms []models.ProgramTermReplaceInput) error {
	current, err := lockProgramTerms(ctx, tx, programID)
	if err != nil {
		return err
	}
	listed := make(map[string]struct{}, len(terms))
	for _, term := range terms {
		if term.ID != "" {
			listed[term.ID] = struct{}{}
		}
	}
	open := 0
	var removed []string
	for id, term := range current {
		if _, ok := listed[id]; !ok {
			removed = append(removed, id)
		} else if term.Status == models.ProgramTermStatusOpen {
			open++
		}
	}
	if len(removed) > 0 {
		var withApplications int
		if err := tx.QueryRow(ctx, `SELECT COUNT(DISTINCT program_term_id) FROM applications WHERE program_term_id = ANY($1::uuid[])`, removed).Scan(&withApplications); err != nil {
			return fmt.Errorf("count applications for removed terms: %w", err)
		}
		if withApplications > 0 {
			return fmt.Errorf("%w: %d removed term(s) have applications", domain.ErrStateLocked, withApplications)
		}
		if _, err := tx.Exec(ctx, `UPDATE program_terms SET status = 'deleted' WHERE id = ANY($1::uuid[])`, removed); err != nil {
			return fmt.Errorf("remove program terms: %w", err)
		}
	}
	now := time.Now()
	added := 0
	for _, term := range terms {
		if term.ID == "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO program_terms (id, program_id, name, status, start_date_time, end_date_time, application_start_date, application_end_date)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
				uuid.NewString(), programID, term.Name, models.ProgramTermStatusOpen,
				term.StartDateTime, term.EndDateTime, term.ApplicationStartDate, term.ApplicationEndDate); err != nil {
				return fmt.Errorf("add program term: %w", err)
			}
			added++
			continue
		}
		existing, ok := current[term.ID]
		if !ok {
			return fmt.Errorf("%w: term %s", domain.ErrProgramTermNotFound, term.ID)
		}
		if term.Matches(existing) {
			continue
		}
		if existing.IsHistorical(now) {
			return fmt.Errorf("%w: historical closed term %s cannot be edited", domain.ErrStateLocked, term.ID)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE program_terms SET name = $2, start_date_time = $3, end_date_time = $4, application_start_date = $5, application_end_date = $6
			WHERE id = $1`,
			term.ID, term.Name, term.StartDateTime, term.EndDateTime, term.ApplicationStartDate, term.ApplicationEndDate); err != nil {
			return fmt.Errorf("update program term %s: %w", term.ID, err)
		}
	}
	if added > 0 && open+added > models.MaxOpenTermsPerProgram {
		return fmt.Errorf("%w: program would have %d open terms (max %d)", domain.ErrStateLocked, open+added, models.MaxOpenTermsPerProgram)
	}
	return nil
}

// Delete removes the program term with the given ID.
func (r *ProgramTermRepository) Delete(ctx context.Context, id string) error {
	ctx, span := programTermTracer.Start(ctx, "db.program_terms.Delete")
	defer span.End()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete program term transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locked int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM program_terms WHERE id = $1 FOR UPDATE`, id).Scan(&locked); errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrProgramTermNotFound
	} else if err != nil {
		return fmt.Errorf("lock program term for delete: %w", err)
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM applications WHERE program_term_id = $1`, id).Scan(&count); err != nil {
		return fmt.Errorf("count applications for term delete: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: term has %d application(s)", domain.ErrStateLocked, count)
	}

	cmd, err := tx.Exec(ctx, `UPDATE program_terms SET status = 'deleted', updated_on = NOW() WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program term: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrProgramTermNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete program term transaction: %w", err)
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

	closed := models.ProgramTermStatusClosed
	updated, err := scanProgramTerm(tx.QueryRow(ctx, `UPDATE program_terms SET status = $2 WHERE id = $1 RETURNING`+programTermCols, id, closed))
	if err != nil {
		return nil, 0, fmt.Errorf("close term: %w", err)
	}
	if err := syncApplicationsWithTermState(ctx, tx, id, closed); err != nil {
		return nil, 0, err
	}
	if err := enqueueProgramIndexByID(ctx, tx, updated.ProgramID); err != nil {
		return nil, 0, err
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
