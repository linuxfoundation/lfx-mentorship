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
	start_date_time, end_date_time, tasks_submitted, admin_notified, attendance_type,
	evaluation, reviewer_note, created_on, updated_on`

func scanApplication(row pgx.Row) (*models.Application, error) {
	var a models.Application
	err := row.Scan(
		&a.ID, &a.ProgramTermID, &a.UserID, &a.Role, &a.Status, &a.ProgramTermStatus,
		&a.StartDateTime, &a.EndDateTime, &a.TasksSubmitted, &a.AdminNotified, &a.AttendanceType,
		&a.Evaluation, &a.ReviewerNote, &a.CreatedOn, &a.UpdatedOn,
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
	if filter.TasksSubmitted != nil {
		args = append(args, *filter.TasksSubmitted)
		where += fmt.Sprintf(` AND tasks_submitted = $%d`, len(args))
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

func (r *ApplicationRepository) ListByProgram(ctx context.Context, programID string, filter models.ProgramApplicationFilter) ([]*models.ProgramApplicationRow, *models.PaginationMeta, error) {
	limit, offset := filter.Limit, filter.Offset
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	args := []any{programID}
	where := ` WHERE pt.program_id = $1 AND a.role = 'mentee'`
	switch filter.Type {
	case models.ProgramApplicationTypeCurrent:
		where += ` AND pt.status = 'open'`
	case models.ProgramApplicationTypePast:
		where += ` AND pt.status = 'closed'`
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND a.status = $%d`, len(args))
	}
	if filter.TermID != "" {
		args = append(args, filter.TermID)
		where += fmt.Sprintf(` AND pt.id = $%d`, len(args))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND (u.name ILIKE $%d OR u.email ILIKE $%d)`, len(args), len(args))
	}
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM applications a JOIN program_terms pt ON pt.id = a.program_term_id JOIN users u ON u.id = a.user_id`+where, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count program applications: %w", err)
	}
	args = append(args, limit, offset)
	q := `SELECT a.user_id, a.id, u.name, u.email, u.avatar_url, a.status, pt.id, pt.name, pt.status,
		(SELECT COUNT(*) FROM tasks t WHERE t.application_id = a.id),
		(SELECT COUNT(*) FROM tasks t WHERE t.application_id = a.id AND t.status IN ('submitted', 'complete')),
		a.reviewer_note, a.created_on, a.updated_on,
		COALESCE((SELECT jsonb_agg(jsonb_build_object('program_id', op.id, 'program_name', op.name, 'status', oa.status))
			FROM applications oa JOIN program_terms ot ON ot.id = oa.program_term_id JOIN programs op ON op.id = ot.program_id
			WHERE oa.user_id = a.user_id AND op.id <> pt.program_id AND oa.role = 'mentee' AND oa.status IN ('pending', 'accepted', 'graduated')), '[]'::jsonb)
		FROM applications a JOIN program_terms pt ON pt.id = a.program_term_id JOIN users u ON u.id = a.user_id` + where + fmt.Sprintf(` ORDER BY a.created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("list program applications: %w", err)
	}
	defer rows.Close()
	result := make([]*models.ProgramApplicationRow, 0)
	for rows.Next() {
		var row models.ProgramApplicationRow
		if err := rows.Scan(&row.UserID, &row.ApplicationID, &row.Name, &row.Email, &row.AvatarURL, &row.Status, &row.Term.ID, &row.Term.Name, &row.Term.Status, &row.TasksTotal, &row.TasksSubmitted, &row.Note, &row.CreatedOn, &row.UpdatedOn, &row.OtherApplications); err != nil {
			return nil, nil, fmt.Errorf("scan program application: %w", err)
		}
		result = append(result, &row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("program application rows: %w", err)
	}
	return result, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
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
	return r.CreateWithTasks(ctx, programTermID, input, nil)
}

func (r *ApplicationRepository) CreateWithTasks(ctx context.Context, programTermID string, input models.ApplicationCreateInput, tasks []models.TaskCreateInput) (*models.Application, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.Create")
	defer span.End()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create application transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO applications (id, program_term_id, user_id, role, status, program_term_status, start_date_time, end_date_time, attendance_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + applicationCols

	a, err := scanApplication(tx.QueryRow(ctx, q,
		input.ID, programTermID, input.UserID, input.Role, input.Status, input.ProgramTermStatus,
		input.StartDateTime, input.EndDateTime, input.AttendanceType,
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create application: %w", err)
	}
	if err := enqueueApplicationMarker(ctx, tx, a, "update_access"); err != nil {
		return nil, err
	}
	if err := insertApplicationTasks(ctx, tx, a, tasks); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create application transaction: %w", err)
	}
	return a, nil
}

func (r *ApplicationRepository) Reapply(ctx context.Context, oldID, programTermID string, input models.ApplicationCreateInput) (*models.Application, error) {
	return r.ReapplyWithTasks(ctx, oldID, programTermID, input, nil)
}

func (r *ApplicationRepository) ReapplyWithTasks(ctx context.Context, oldID, programTermID string, input models.ApplicationCreateInput, tasks []models.TaskCreateInput) (*models.Application, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin reapply transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var taskIDs []string
	rows, err := tx.Query(ctx, `SELECT id FROM tasks WHERE application_id = $1`, oldID)
	if err != nil {
		return nil, fmt.Errorf("list withdrawn application tasks: %w", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		taskIDs = append(taskIDs, id)
	}
	rows.Close()
	if _, err := tx.Exec(ctx, `DELETE FROM applications WHERE id = $1`, oldID); err != nil {
		return nil, fmt.Errorf("delete withdrawn application: %w", err)
	}
	const q = `INSERT INTO applications (id, program_term_id, user_id, role, status, program_term_status, start_date_time, end_date_time, attendance_type) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING ` + applicationCols
	a, err := scanApplication(tx.QueryRow(ctx, q, input.ID, programTermID, input.UserID, input.Role, input.Status, input.ProgramTermStatus, input.StartDateTime, input.EndDateTime, input.AttendanceType))
	if err != nil {
		return nil, fmt.Errorf("create replacement application: %w", err)
	}
	if err := enqueueObjectDeleteMarker(ctx, tx, "mentorship_application", oldID); err != nil {
		return nil, err
	}
	for _, taskID := range taskIDs {
		if err := enqueueObjectDeleteMarker(ctx, tx, "mentorship_task", taskID); err != nil {
			return nil, err
		}
	}
	if err := enqueueApplicationMarker(ctx, tx, a, "update_access"); err != nil {
		return nil, err
	}
	if err := insertApplicationTasks(ctx, tx, a, tasks); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reapply transaction: %w", err)
	}
	return a, nil
}

func insertApplicationTasks(ctx context.Context, tx pgx.Tx, application *models.Application, tasks []models.TaskCreateInput) error {
	for _, task := range tasks {
		created, err := scanTask(tx.QueryRow(ctx, `
			INSERT INTO tasks (id, application_id, program_term_id, assignee_id, owner_id, name, description, category, status, application_status, program_term_status, custom, submit_file, due_date, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			RETURNING `+taskCols,
			task.ID, application.ID, task.ProgramTermID, task.AssigneeID, task.OwnerID, task.Name, task.Description,
			task.Category, task.Status, application.Status, application.ProgramTermStatus, task.Custom, task.SubmitFile, task.DueDate, task.CreatedBy))
		if err != nil {
			name := ""
			if task.Name != nil {
				name = *task.Name
			}
			return fmt.Errorf("create prerequisite task %q: %w", name, err)
		}
		if err := enqueueTaskMarker(ctx, tx, created, "update_access"); err != nil {
			name := ""
			if created.Name != nil {
				name = *created.Name
			}
			return fmt.Errorf("enqueue prerequisite task %q: %w", name, err)
		}
	}
	return nil
}

// Update patches an application's status fields.
func (r *ApplicationRepository) Update(ctx context.Context, id string, input models.ApplicationUpdateInput) (*models.Application, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.application_id", id))
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update application transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		UPDATE applications SET
			status              = COALESCE($2, status),
			program_term_status = COALESCE($3, program_term_status),
			start_date_time     = COALESCE($4, start_date_time),
			end_date_time       = COALESCE($5, end_date_time),
			tasks_submitted     = COALESCE($6, tasks_submitted),
			admin_notified      = COALESCE($7, admin_notified),
			attendance_type     = COALESCE($8, attendance_type),
			evaluation          = COALESCE($9, evaluation),
			reviewer_note       = COALESCE($10, reviewer_note)
		WHERE id = $1
		RETURNING ` + applicationCols

	a, err := scanApplication(tx.QueryRow(ctx, q,
		id, input.Status, input.ProgramTermStatus, input.StartDateTime, input.EndDateTime,
		input.TasksSubmitted, input.AdminNotified, input.AttendanceType, input.Evaluation, input.ReviewerNote,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrApplicationNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update application: %w", err)
	}
	if a.Role == models.ApplicationRoleMentor && a.Status == models.ApplicationStatusAccepted {
		if err := ensureAcceptedMentorMembership(ctx, tx, a); err != nil {
			return nil, err
		}
	}
	if err := enqueueApplicationMarker(ctx, tx, a, "update_access"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update application transaction: %w", err)
	}
	return a, nil
}

func ensureAcceptedMentorMembership(ctx context.Context, tx pgx.Tx, application *models.Application) error {
	var programID string
	if err := tx.QueryRow(ctx, `SELECT program_id FROM program_terms WHERE id = $1`, application.ProgramTermID).Scan(&programID); err != nil {
		return fmt.Errorf("resolve accepted mentor program: %w", err)
	}

	const q = `
		INSERT INTO program_members (id, program_id, user_id, member_type, status)
		VALUES (gen_random_uuid(), $1, $2, 'mentor', 'active')
		ON CONFLICT (program_id, user_id, member_type)
		DO UPDATE SET status = 'active', updated_on = NOW()
		RETURNING ` + programMemberCols
	member, err := scanProgramMember(tx.QueryRow(ctx, q, programID, application.UserID))
	if err != nil {
		return fmt.Errorf("upsert accepted mentor membership: %w", err)
	}
	if err := enqueueMemberMarker(ctx, tx, member, "put"); err != nil {
		return fmt.Errorf("enqueue accepted mentor membership: %w", err)
	}
	return nil
}

// Delete removes an application.
func (r *ApplicationRepository) Delete(ctx context.Context, id string) error {
	ctx, span := applicationTracer.Start(ctx, "db.applications.Delete")
	defer span.End()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete application transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationCols+` FROM applications WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrApplicationNotFound
	}
	if err != nil {
		return fmt.Errorf("load application before delete: %w", err)
	}
	var taskIDs []string
	rows, err := tx.Query(ctx, `SELECT id FROM tasks WHERE application_id = $1`, id)
	if err != nil {
		return fmt.Errorf("list application tasks before delete: %w", err)
	}
	for rows.Next() {
		var taskID string
		if err := rows.Scan(&taskID); err != nil {
			rows.Close()
			return fmt.Errorf("scan application task before delete: %w", err)
		}
		taskIDs = append(taskIDs, taskID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate application tasks before delete: %w", err)
	}
	rows.Close()

	cmd, err := tx.Exec(ctx, `DELETE FROM applications WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete application: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrApplicationNotFound
	}
	if err := enqueueObjectDeleteMarker(ctx, tx, "mentorship_application", current.ID); err != nil {
		return err
	}
	for _, taskID := range taskIDs {
		if err := enqueueObjectDeleteMarker(ctx, tx, "mentorship_task", taskID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete application transaction: %w", err)
	}
	return nil
}

func enqueueObjectDeleteMarker(ctx context.Context, tx pgx.Tx, objectType, objectID string) error {
	const q = `
		INSERT INTO fga_outbox (marker_kind, object_type, object_uid, desired_operation)
		VALUES ('object', $1, $2, 'delete_access')
		ON CONFLICT (object_type, object_uid) WHERE marker_kind = 'object'
		DO UPDATE SET desired_operation = EXCLUDED.desired_operation,
		              generation = fga_outbox.generation + 1,
		             state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END, claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END, claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
		              attempts = 0, next_attempt_at = NOW(), last_error = NULL,
		              updated_on = NOW()`
	if _, err := tx.Exec(ctx, q, objectType, objectID); err != nil {
		return fmt.Errorf("enqueue %s delete marker: %w", objectType, err)
	}
	return nil
}

func enqueueApplicationMarker(ctx context.Context, tx pgx.Tx, application *models.Application, operation string) error {
	var lfid string
	const lookup = `
		SELECT u.lfid
		FROM program_terms pt
		JOIN users u ON u.id = $2
		WHERE pt.id = $1`
	if err := tx.QueryRow(ctx, lookup, application.ProgramTermID, application.UserID).Scan(&lfid); err != nil {
		return fmt.Errorf("resolve application FGA parents: %w", err)
	}
	if lfid == "" {
		return fmt.Errorf("application user %s has no LFID", application.UserID)
	}
	const q = `
		INSERT INTO fga_outbox (marker_kind, object_type, object_uid, desired_operation)
		VALUES ('object', 'mentorship_application', $1, $2)
		ON CONFLICT (object_type, object_uid) WHERE marker_kind = 'object'
		DO UPDATE SET desired_operation = EXCLUDED.desired_operation,
		              generation = fga_outbox.generation + 1,
		             state = CASE WHEN fga_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END, claimed_generation = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_generation ELSE NULL END, claimed_at = CASE WHEN fga_outbox.state = 'in_flight' THEN fga_outbox.claimed_at ELSE NULL END,
		              attempts = 0, next_attempt_at = NOW(), last_error = NULL,
		              updated_on = NOW()`
	if _, err := tx.Exec(ctx, q, application.ID, operation); err != nil {
		return fmt.Errorf("enqueue application FGA marker: %w", err)
	}
	return nil
}

// CountBlockingAppsForProgram returns applications in a non-terminal state across all terms of a program.
func (r *ApplicationRepository) CountBlockingAppsForProgram(ctx context.Context, programID string) (int, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.CountBlockingAppsForProgram")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", programID))

	var count int
	// NOTE: status literals must stay in sync with models.ApplicationStatus.
	const q = `
		SELECT COUNT(*) FROM applications a
		JOIN program_terms pt ON pt.id = a.program_term_id
		WHERE pt.program_id = $1
		AND a.status IN ('pending', 'accepted', 'graduated')`
	if err := r.pool.QueryRow(ctx, q, programID).Scan(&count); err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("count blocking applications: %w", err)
	}
	return count, nil
}

// CountAcceptedByTerm returns the count of accepted applications for a term.
func (r *ApplicationRepository) CountAcceptedByTerm(ctx context.Context, termID string) (int, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.CountAcceptedByTerm")
	defer span.End()
	span.SetAttributes(attribute.String("db.term_id", termID))

	var count int
	// NOTE: status literals must stay in sync with models.ApplicationStatus.
	const q = `SELECT COUNT(*) FROM applications WHERE program_term_id = $1 AND status = 'accepted'`
	if err := r.pool.QueryRow(ctx, q, termID).Scan(&count); err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("count accepted applications: %w", err)
	}
	return count, nil
}

// FindByTermAndUser returns an application for a specific term and user, or nil.
func (r *ApplicationRepository) FindByTermAndUser(ctx context.Context, termID, userID string) (*models.Application, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.FindByTermAndUser")
	defer span.End()

	q := `SELECT ` + applicationCols + ` FROM applications WHERE program_term_id = $1 AND user_id = $2 LIMIT 1`
	a, err := scanApplication(r.pool.QueryRow(ctx, q, termID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("find application by term and user: %w", err)
	}
	return a, nil
}

// BulkDeclineByTerm moves all pending/submitted applications in a term to declined.
func (r *ApplicationRepository) BulkDeclineByTerm(ctx context.Context, termID string) (int, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.BulkDeclineByTerm")
	defer span.End()
	span.SetAttributes(attribute.String("db.term_id", termID))
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin bulk decline transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// NOTE: status literals must stay in sync with models.ApplicationStatus.
	const q = `
		UPDATE applications SET status = 'declined'
		WHERE program_term_id = $1 AND status = 'pending'
		RETURNING ` + applicationCols
	rows, err := tx.Query(ctx, q, termID)
	if err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("bulk decline: %w", err)
	}
	defer rows.Close()

	var count int
	applications := make([]*models.Application, 0)
	for rows.Next() {
		application, scanErr := scanApplication(rows)
		if scanErr != nil {
			return 0, fmt.Errorf("scan bulk-declined application: %w", scanErr)
		}
		applications = append(applications, application)
		count++
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("bulk decline rows: %w", err)
	}
	rows.Close()
	for _, application := range applications {
		if err := enqueueApplicationMarker(ctx, tx, application, "update_access"); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit bulk decline transaction: %w", err)
	}
	return count, nil
}

// ListPastMenteesByTerm returns accepted/graduated applications for a term.
func (r *ApplicationRepository) ListPastMenteesByTerm(ctx context.Context, termID string) ([]*models.Application, error) {
	ctx, span := applicationTracer.Start(ctx, "db.applications.ListPastMenteesByTerm")
	defer span.End()
	span.SetAttributes(attribute.String("db.term_id", termID))

	// NOTE: status literals must stay in sync with models.ApplicationStatus.
	q := `SELECT ` + applicationCols + ` FROM applications WHERE program_term_id = $1 AND status IN ('accepted', 'graduated') ORDER BY created_on`
	rows, err := r.pool.Query(ctx, q, termID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list past mentees: %w", err)
	}
	defer rows.Close()

	var apps []*models.Application
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan past mentee: %w", err)
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("past mentees rows: %w", err)
	}
	if apps == nil {
		apps = []*models.Application{}
	}
	return apps, nil
}
