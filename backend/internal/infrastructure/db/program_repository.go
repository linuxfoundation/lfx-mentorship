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

var programTracer = otel.Tracer("programs-db")

// ProgramRepository implements domain.ProgramRepository against PostgreSQL.
type ProgramRepository struct {
	pool *pgxpool.Pool
}

// NewProgramRepository creates a new ProgramRepository.
func NewProgramRepository(pool *pgxpool.Pool) *ProgramRepository {
	return &ProgramRepository{pool: pool}
}

const programCols = `
	id, name, slug, status, is_paid, description, logo_url, website_url, repo_link,
	code_of_conduct, industry, color, lfid, cii_project_id, accept_applications,
	terms_and_conditions, program_term_status, discover_sort_rank, amount_raised,
	term_type_fall, term_type_spring, term_type_summer, term_type_ongoing, term_type_custom,
	task_templates, created_on, updated_on`

func scanProgram(row pgx.Row) (*models.Program, error) {
	var p models.Program
	err := row.Scan(
		&p.ID, &p.Name, &p.Slug, &p.Status, &p.IsPaid, &p.Description, &p.LogoURL,
		&p.WebsiteURL, &p.RepoLink, &p.CodeOfConduct, &p.Industry, &p.Color, &p.LFID,
		&p.CIIProjectID, &p.AcceptApplications, &p.TermsAndConditions, &p.ProgramTermStatus,
		&p.DiscoverSortRank, &p.AmountRaised,
		&p.TermTypeFall, &p.TermTypeSpring, &p.TermTypeSummer, &p.TermTypeOngoing, &p.TermTypeCustom,
		&p.TaskTemplates,
		&p.CreatedOn, &p.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetByID returns the program with the given UUID or ErrProgramNotFound.
func (r *ProgramRepository) GetByID(ctx context.Context, id string) (*models.Program, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", id))

	q := `SELECT` + programCols + ` FROM programs WHERE id = $1`
	p, err := scanProgram(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program by id: %w", err)
	}
	return p, nil
}

// GetBySlug returns the program with the given slug or ErrProgramNotFound.
func (r *ProgramRepository) GetBySlug(ctx context.Context, slug string) (*models.Program, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.GetBySlug")
	defer span.End()
	span.SetAttributes(attribute.String("db.slug", slug))

	q := `SELECT` + programCols + ` FROM programs WHERE slug = $1`
	p, err := scanProgram(r.pool.QueryRow(ctx, q, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program by slug: %w", err)
	}
	return p, nil
}

// List returns a paginated slice of programs optionally filtered by status or search.
func (r *ProgramRepository) List(ctx context.Context, filter models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.List")
	defer span.End()

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{}
	where := ` WHERE 1=1`
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND name ILIKE $%d`, len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM programs`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count programs: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT` + programCols + ` FROM programs` + where +
		fmt.Sprintf(` ORDER BY discover_sort_rank DESC, created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list programs: %w", err)
	}
	defer rows.Close()

	var programs []*models.Program
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan program: %w", err)
		}
		programs = append(programs, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	if programs == nil {
		programs = []*models.Program{}
	}
	return programs, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// Create inserts a new program and returns the persisted record.
func (r *ProgramRepository) Create(ctx context.Context, input models.ProgramCreateInput) (*models.Program, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.Create")
	defer span.End()

	const q = `
		INSERT INTO programs (
			id, name, slug, status, is_paid, description, logo_url, website_url, repo_link,
			code_of_conduct, industry, color, lfid, cii_project_id, accept_applications,
			terms_and_conditions, term_type_fall, term_type_spring, term_type_summer,
			term_type_ongoing, term_type_custom, task_templates
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		RETURNING` + programCols

	p, err := scanProgram(r.pool.QueryRow(ctx, q,
		input.ID, input.Name, input.Slug, input.Status, input.IsPaid,
		input.Description, input.LogoURL, input.WebsiteURL, input.RepoLink,
		input.CodeOfConduct, input.Industry, input.Color, input.LFID, input.CIIProjectID,
		input.AcceptApplications, input.TermsAndConditions,
		input.TermTypeFall, input.TermTypeSpring, input.TermTypeSummer,
		input.TermTypeOngoing, input.TermTypeCustom,
		nilIfEmpty(input.TaskTemplates),
	))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program: %w", err)
	}
	return p, nil
}

// Update patches the program and returns the updated record.
func (r *ProgramRepository) Update(ctx context.Context, id string, input models.ProgramUpdateInput) (*models.Program, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.Update")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", id))

	const q = `
		UPDATE programs SET
			name                = COALESCE($2,  name),
			slug                = COALESCE($3,  slug),
			status              = COALESCE($4,  status),
			is_paid             = COALESCE($5,  is_paid),
			description         = COALESCE($6,  description),
			logo_url            = COALESCE($7,  logo_url),
			website_url         = COALESCE($8,  website_url),
			repo_link           = COALESCE($9,  repo_link),
			code_of_conduct     = COALESCE($10, code_of_conduct),
			industry            = COALESCE($11, industry),
			color               = COALESCE($12, color),
			lfid                = COALESCE($13, lfid),
			cii_project_id      = COALESCE($14, cii_project_id),
			accept_applications = COALESCE($15, accept_applications),
			terms_and_conditions= COALESCE($16, terms_and_conditions),
			program_term_status = COALESCE($17, program_term_status),
			discover_sort_rank  = COALESCE($18, discover_sort_rank),
			term_type_fall      = COALESCE($19, term_type_fall),
			term_type_spring    = COALESCE($20, term_type_spring),
			term_type_summer    = COALESCE($21, term_type_summer),
			term_type_ongoing   = COALESCE($22, term_type_ongoing),
			term_type_custom    = COALESCE($23, term_type_custom),
			task_templates      = COALESCE($24, task_templates)
		WHERE id = $1
		RETURNING` + programCols

	var statusVal *string
	if input.Status != nil {
		s := string(*input.Status)
		statusVal = &s
	}

	p, err := scanProgram(r.pool.QueryRow(ctx, q,
		id, input.Name, input.Slug, statusVal, input.IsPaid,
		input.Description, input.LogoURL, input.WebsiteURL, input.RepoLink,
		input.CodeOfConduct, input.Industry, input.Color, input.LFID, input.CIIProjectID,
		input.AcceptApplications, input.TermsAndConditions, input.ProgramTermStatus, input.DiscoverSortRank,
		input.TermTypeFall, input.TermTypeSpring, input.TermTypeSummer, input.TermTypeOngoing, input.TermTypeCustom,
		nilIfEmpty(input.TaskTemplates),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program: %w", err)
	}
	return p, nil
}

// Delete removes the program with the given ID.
func (r *ProgramRepository) Delete(ctx context.Context, id string) error {
	ctx, span := programTracer.Start(ctx, "db.programs.Delete")
	defer span.End()

	cmd, err := r.pool.Exec(ctx, `DELETE FROM programs WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrProgramNotFound
	}
	return nil
}

// ListSkills returns all skills for a program.
func (r *ProgramRepository) ListSkills(ctx context.Context, programID string) ([]*models.ProgramSkill, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.ListSkills")
	defer span.End()

	rows, err := r.pool.Query(ctx,
		`SELECT id, program_id, skill, created_on, updated_on FROM program_skills WHERE program_id = $1 ORDER BY skill`,
		programID,
	)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

	var skills []*models.ProgramSkill
	for rows.Next() {
		var s models.ProgramSkill
		if err := rows.Scan(&s.ID, &s.ProgramID, &s.Skill, &s.CreatedOn, &s.UpdatedOn); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		skills = append(skills, &s)
	}
	if skills == nil {
		skills = []*models.ProgramSkill{}
	}
	return skills, rows.Err()
}

// AddSkill inserts a new skill for a program.
func (r *ProgramRepository) AddSkill(ctx context.Context, programID string, input models.ProgramSkillCreateInput) (*models.ProgramSkill, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.AddSkill")
	defer span.End()

	var s models.ProgramSkill
	err := r.pool.QueryRow(ctx,
		`INSERT INTO program_skills (program_id, skill) VALUES ($1, $2)
		 ON CONFLICT (program_id, skill) DO UPDATE SET skill = EXCLUDED.skill
		 RETURNING id, program_id, skill, created_on, updated_on`,
		programID, input.Skill,
	).Scan(&s.ID, &s.ProgramID, &s.Skill, &s.CreatedOn, &s.UpdatedOn)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("add skill: %w", err)
	}
	return &s, nil
}

// DeleteSkill removes a skill by its ID.
func (r *ProgramRepository) DeleteSkill(ctx context.Context, skillID string) error {
	ctx, span := programTracer.Start(ctx, "db.programs.DeleteSkill")
	defer span.End()

	cmd, err := r.pool.Exec(ctx, `DELETE FROM program_skills WHERE id = $1`, skillID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete skill: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrProgramNotFound
	}
	return nil
}

// GetFundingStats returns funding stats for a program.
func (r *ProgramRepository) GetFundingStats(ctx context.Context, programID string) (*models.ProgramFundingStats, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.GetFundingStats")
	defer span.End()

	var fs models.ProgramFundingStats
	err := r.pool.QueryRow(ctx,
		`SELECT id, program_id, amount_raised, created_on, updated_on FROM program_funding_stats WHERE program_id = $1`,
		programID,
	).Scan(&fs.ID, &fs.ProgramID, &fs.AmountRaised, &fs.CreatedOn, &fs.UpdatedOn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get funding stats: %w", err)
	}
	return &fs, nil
}
