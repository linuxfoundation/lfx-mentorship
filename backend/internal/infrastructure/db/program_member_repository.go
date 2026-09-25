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

// FindActiveReviewerByProgramAndUser returns an active mentor or program admin membership.
func (r *ProgramMemberRepository) FindActiveReviewerByProgramAndUser(ctx context.Context, programID, userID string) (*models.ProgramMember, error) {
	return r.findActiveMembership(ctx, programID, userID, "member_type IN ('mentor', 'program_admin')", "find active reviewer membership")
}

// FindActiveProgramAdminByProgramAndUser returns an active program admin membership.
func (r *ProgramMemberRepository) FindActiveProgramAdminByProgramAndUser(ctx context.Context, programID, userID string) (*models.ProgramMember, error) {
	return r.findActiveMembership(ctx, programID, userID, "member_type = 'program_admin'", "find active program admin membership")
}

func (r *ProgramMemberRepository) findActiveMembership(ctx context.Context, programID, userID, rolePredicate, operation string) (*models.ProgramMember, error) {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members."+operation)
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", programID), attribute.String("db.user_id", userID))

	q := `SELECT ` + programMemberCols + ` FROM program_members WHERE program_id = $1 AND user_id = $2 AND status = 'active' AND ` + rolePredicate + ` ORDER BY created_on DESC LIMIT 1`
	m, err := scanProgramMember(r.pool.QueryRow(ctx, q, programID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramMemberNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("%s: %w", operation, err)
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

func (r *ProgramMemberRepository) ListMentorManagement(ctx context.Context, programID string, filter models.ProgramMemberFilter) ([]*models.ProgramMentorManagementRow, *models.PaginationMeta, error) {
	limit, offset := filter.Limit, filter.Offset
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args := []any{programID}
	where := ` WHERE pm.program_id = $1 AND pm.member_type = 'mentor'`
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND pm.status = $%d`, len(args))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND (u.name ILIKE $%d OR u.email ILIKE $%d OR u.lfid ILIKE $%d)`, len(args), len(args), len(args))
	}
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM program_members pm JOIN users u ON u.id = pm.user_id`+where, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count mentor management: %w", err)
	}
	args = append(args, limit, offset)
	q := `SELECT pm.id, pm.user_id, u.name, COALESCE(pm.email, u.email), u.lfid, u.avatar_url, pm.status, pm.created_on, pm.updated_on, EXISTS(SELECT 1 FROM user_profiles up WHERE up.user_id = pm.user_id AND up.profile_type = 'mentor')` + ` FROM program_members pm JOIN users u ON u.id = pm.user_id` + where + fmt.Sprintf(` ORDER BY pm.created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("list mentor management: %w", err)
	}
	defer rows.Close()
	result := make([]*models.ProgramMentorManagementRow, 0)
	for rows.Next() {
		var row models.ProgramMentorManagementRow
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.Email, &row.Username, &row.AvatarURL, &row.Status, &row.CreatedOn, &row.UpdatedOn, &row.ProfileCreated); err != nil {
			return nil, nil, fmt.Errorf("scan mentor management: %w", err)
		}
		result = append(result, &row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return result, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// Create adds a member to a program.
func (r *ProgramMemberRepository) Create(ctx context.Context, programID string, input models.ProgramMemberCreateInput) (*models.ProgramMember, error) {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members.Create")
	defer span.End()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create program member transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO program_members (id, program_id, user_id, member_type, status, email)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + programMemberCols

	m, err := scanProgramMember(tx.QueryRow(ctx, q,
		input.ID, programID, input.UserID, input.MemberType, input.Status, input.Email,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program member: %w", err)
	}
	if isActiveMember(m) {
		if err := enqueueMemberMarker(ctx, tx, m, "put"); err != nil {
			return nil, err
		}
		if m.MemberType == models.MemberTypeMentor {
			if err := enqueueProgramIndexByID(ctx, tx, m.ProgramID); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create program member transaction: %w", err)
	}
	return m, nil
}

// Update patches a program member's status or email.
func (r *ProgramMemberRepository) Update(ctx context.Context, id string, input models.ProgramMemberUpdateInput) (*models.ProgramMember, error) {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.member_id", id))
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update program member transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanProgramMember(tx.QueryRow(ctx, `SELECT `+programMemberCols+` FROM program_members WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramMemberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load program member before update: %w", err)
	}

	const q = `
		UPDATE program_members SET
			status = COALESCE($2, status),
			email  = COALESCE($3, email)
		WHERE id = $1
		RETURNING ` + programMemberCols

	m, err := scanProgramMember(tx.QueryRow(ctx, q, id, input.Status, input.Email))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramMemberNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program member: %w", err)
	}
	if isActiveMember(current) || isActiveMember(m) {
		op := "remove"
		if isActiveMember(m) {
			op = "put"
		}
		if err := enqueueMemberMarker(ctx, tx, m, op); err != nil {
			return nil, err
		}
		if current.MemberType == models.MemberTypeMentor || m.MemberType == models.MemberTypeMentor {
			if err := enqueueProgramIndexByID(ctx, tx, m.ProgramID); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update program member transaction: %w", err)
	}
	return m, nil
}

// Delete removes a program member.
func (r *ProgramMemberRepository) Delete(ctx context.Context, id string) error {
	ctx, span := programMemberTracer.Start(ctx, "db.program_members.Delete")
	defer span.End()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete program member transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := scanProgramMember(tx.QueryRow(ctx, `SELECT `+programMemberCols+` FROM program_members WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrProgramMemberNotFound
	}
	if err != nil {
		return fmt.Errorf("load program member before delete: %w", err)
	}

	cmd, err := tx.Exec(ctx, `DELETE FROM program_members WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program member: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrProgramMemberNotFound
	}
	if isActiveMember(current) {
		if err := enqueueMemberMarker(ctx, tx, current, "remove"); err != nil {
			return err
		}
		if current.MemberType == models.MemberTypeMentor {
			if err := enqueueProgramIndexByID(ctx, tx, current.ProgramID); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete program member transaction: %w", err)
	}
	return nil
}

func isActiveMember(member *models.ProgramMember) bool {
	return member != nil && member.Status != nil && *member.Status == models.ProgramMemberStatusActive
}

func enqueueMemberMarker(ctx context.Context, tx pgx.Tx, member *models.ProgramMember, operation string) error {
	var lfid *string
	if err := tx.QueryRow(ctx, `SELECT lfid FROM users WHERE id = $1`, member.UserID).Scan(&lfid); err != nil {
		return fmt.Errorf("resolve program member LFID: %w", err)
	}
	if lfid == nil || *lfid == "" {
		return fmt.Errorf("program member user %s has no LFID", member.UserID)
	}
	relation := "mentor"
	if member.MemberType == models.MemberTypeProgramAdmin {
		relation = "writer"
	}
	markerOperation := "sync"
	if operation == "remove" {
		markerOperation = "remove"
	}
	const q = `
		INSERT INTO fga_outbox (marker_kind, object_type, object_uid, relation, username, desired_operation)
		VALUES ('membership', 'mentorship_program', $1, $2, $3, $4)
		ON CONFLICT (object_type, object_uid, relation, username) WHERE marker_kind = 'membership'
		DO UPDATE SET desired_operation = EXCLUDED.desired_operation, generation = fga_outbox.generation + 1,
		              state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END, claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END, claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
		              attempts = 0, next_attempt_at = NOW(), last_error = NULL,
		              updated_on = NOW()`
	if _, err := tx.Exec(ctx, q, member.ProgramID, relation, *lfid, markerOperation); err != nil {
		return fmt.Errorf("enqueue program member FGA marker: %w", err)
	}
	return nil
}
