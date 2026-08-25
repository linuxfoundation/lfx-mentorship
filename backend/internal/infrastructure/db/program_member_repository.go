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

var programMemberTracer = otel.Tracer("program-members-db")

// ProgramMemberRepository implements domain.ProgramMemberRepository against PostgreSQL.
type ProgramMemberRepository struct {
	pool *pgxpool.Pool
}

// NewProgramMemberRepository creates a new ProgramMemberRepository.
func NewProgramMemberRepository(pool *pgxpool.Pool) *ProgramMemberRepository {
	return &ProgramMemberRepository{pool: pool}
}

const programMemberCols = `id, program_id, user_id, member_type, status, email, created_on, updated_on`

func scanProgramMember(row pgx.Row) (*models.ProgramMember, error) {
	var m models.ProgramMember
	err := row.Scan(&m.ID, &m.ProgramID, &m.UserID, &m.MemberType, &m.Status, &m.Email, &m.CreatedOn, &m.UpdatedOn)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetByID returns the program member with the given UUID or ErrProgramMemberNotFound.
func (r *ProgramMemberRepository) GetByID(ctx context.Context, id string) (*models.ProgramMember, error) {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("db.member_id", id))

	q := `SELECT ` + programMemberCols + ` FROM program_members WHERE id = $1`
	m, err := scanProgramMember(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramMemberNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program member by id: %w", err)
	}
	return m, nil
}

// FindByProgramAndUser returns the member record for a given program + user pair.
func (r *ProgramMemberRepository) FindByProgramAndUser(ctx context.Context, programID, userID string) (*models.ProgramMember, error) {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members.FindByProgramAndUser")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", programID), attribute.String("db.user_id", userID))

	q := `SELECT ` + programMemberCols + ` FROM program_members WHERE program_id = $1 AND user_id = $2`
	m, err := scanProgramMember(r.pool.QueryRow(ctx, q, programID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramMemberNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("find program member by program and user: %w", err)
	}
	return m, nil
}

// ListByProgram returns paginated members for a program.
func (r *ProgramMemberRepository) ListByProgram(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMember, *models.PaginationMeta, error) {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members.ListByProgram")
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
	if filter.MemberType != "" {
		args = append(args, filter.MemberType)
		where += fmt.Sprintf(` AND member_type = $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM program_members`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count program members: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT ` + programMemberCols + ` FROM program_members` + where +
		fmt.Sprintf(` ORDER BY created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list program members: %w", err)
	}
	defer rows.Close()

	var members []*models.ProgramMember
	for rows.Next() {
		m, err := scanProgramMember(rows)
		if err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan program member: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	if members == nil {
		members = []*models.ProgramMember{}
	}
	return members, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// Create adds a member to a program.
func (r *ProgramMemberRepository) Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members.Create")
	defer span.End()

	const q = `
		INSERT INTO program_members (id, program_id, user_id, member_type, status, email)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + programMemberCols

	m, err := scanProgramMember(r.pool.QueryRow(ctx, q,
		input.ID, programID, input.UserID, input.MemberType, input.Status, input.Email,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program member: %w", err)
	}
	return m, nil
}

// Update patches a program member's status or email.
func (r *ProgramMemberRepository) Update(ctx context.Context, id string, input models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.member_id", id))

	const q = `
		UPDATE program_members SET
			status = COALESCE($2, status),
			email  = COALESCE($3, email)
		WHERE id = $1
		RETURNING ` + programMemberCols

	m, err := scanProgramMember(r.pool.QueryRow(ctx, q, id, input.Status, input.Email))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramMemberNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program member: %w", err)
	}
	return m, nil
}

// Delete removes a program member.
func (r *ProgramMemberRepository) Delete(ctx context.Context, id string) error {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members.Delete")
	defer span.End()

	cmd, err := r.pool.Exec(ctx, `DELETE FROM program_members WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program member: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrProgramMemberNotFound
	}
	return nil
}
