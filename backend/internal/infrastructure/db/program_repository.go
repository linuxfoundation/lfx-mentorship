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

const programSelectCols = `
	programs.id, programs.name, programs.slug, programs.status, programs.is_paid,
	programs.description, programs.logo_url, programs.website_url, programs.repo_link,
	programs.code_of_conduct, programs.industry, programs.color, programs.lfid,
	programs.cii_project_id, programs.accept_applications,
	programs.terms_and_conditions, programs.program_term_status, programs.discover_sort_rank,
	COALESCE(pfs.amount_raised, programs.amount_raised) AS amount_raised,
	programs.mentee_needs, programs.task_templates, programs.created_on, programs.updated_on`

const programReturningCols = `
	id, name, slug, status, is_paid, description, logo_url, website_url, repo_link,
	code_of_conduct, industry, color, lfid, cii_project_id, accept_applications,
	terms_and_conditions, program_term_status, discover_sort_rank, amount_raised,
	mentee_needs, task_templates, created_on, updated_on`

const programsWithFundingFrom = `
	FROM programs
	LEFT JOIN program_funding_stats pfs ON pfs.program_id = programs.id`

func scanProgram(row pgx.Row) (*models.Program, error) {
	var p models.Program
	err := row.Scan(
		&p.ID, &p.Name, &p.Slug, &p.Status, &p.IsPaid, &p.Description, &p.LogoURL,
		&p.WebsiteURL, &p.RepoLink, &p.CodeOfConduct, &p.Industry, &p.Color, &p.LFID,
		&p.CIIProjectID, &p.AcceptApplications, &p.TermsAndConditions, &p.ProgramTermStatus,
		&p.DiscoverSortRank, &p.AmountRaised, &p.MenteeNeeds, &p.TaskTemplates,
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

	q := `SELECT` + programSelectCols + programsWithFundingFrom + ` WHERE programs.id = $1`
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

	q := `SELECT` + programSelectCols + programsWithFundingFrom + ` WHERE programs.slug = $1`
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
		where += fmt.Sprintf(` AND programs.status = $%d`, len(args))
	} else {
		where += ` AND programs.status = 'published'` // public list only shows published programs
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND programs.name ILIKE $%d`, len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM programs`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count programs: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT` + programSelectCols + programsWithFundingFrom + where +
		fmt.Sprintf(` ORDER BY programs.discover_sort_rank DESC, programs.created_on DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

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

func catalogLimitOffset(filter models.ProgramFilter) (limit, offset int) {
	limit = filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset = filter.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// Term-derived discovery predicates, matching ProgramTerm.DiscoveryLabel / public catalog status.
const (
	sqlHasAcceptingTerm = `EXISTS (
		SELECT 1 FROM program_terms pt
		WHERE pt.program_id = programs.id
		  AND pt.status = 'open'
		  AND pt.application_start_date IS NOT NULL
		  AND pt.application_end_date IS NOT NULL
		  AND NOW() BETWEEN pt.application_start_date AND pt.application_end_date
	)`
	sqlHasComingSoonTerm = `EXISTS (
		SELECT 1 FROM program_terms pt
		WHERE pt.program_id = programs.id
		  AND pt.status = 'open'
		  AND pt.application_start_date IS NOT NULL
		  AND pt.application_end_date IS NOT NULL
		  AND pt.application_start_date > NOW()
	)`
	sqlHasOpenTerm = `EXISTS (
		SELECT 1 FROM program_terms pt
		WHERE pt.program_id = programs.id
		  AND pt.status = 'open'
	)`
)

func catalogWhere(filter models.ProgramFilter) (where string, args []any) {
	args = []any{}
	where = ` WHERE 1=1`
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(` AND programs.status = $%d`, len(args))
	} else {
		where += ` AND programs.status = 'published'`
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		n := len(args)
		where += fmt.Sprintf(` AND programs.name ILIKE $%d`, n)
	}
	if filter.Skill != "" {
		args = append(args, filter.Skill)
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM program_skills ps
			WHERE ps.program_id = programs.id AND LOWER(ps.skill) = LOWER($%d)
		)`, len(args))
	}
	switch filter.DiscoveryStatus {
	case "acceptance":
		where += ` AND ` + sqlHasAcceptingTerm
	case "in-progress":
		where += ` AND ` + sqlHasOpenTerm + ` AND NOT ` + sqlHasAcceptingTerm
	case "completed":
		where += ` AND NOT ` + sqlHasOpenTerm
	}
	return where, args
}

func catalogOrderBy(sortBy string) string {
	acceptingRank := `CASE WHEN ` + sqlHasAcceptingTerm + ` THEN 0 WHEN ` + sqlHasComingSoonTerm + ` THEN 1 WHEN ` + sqlHasOpenTerm + ` THEN 2 ELSE 3 END`
	completedRank := `CASE WHEN NOT (` + sqlHasOpenTerm + `) THEN 0 WHEN ` + sqlHasOpenTerm + ` AND NOT (` + sqlHasAcceptingTerm + `) AND NOT (` + sqlHasComingSoonTerm + `) THEN 1 WHEN ` + sqlHasAcceptingTerm + ` THEN 2 ELSE 3 END`
	switch sortBy {
	case "name_asc":
		return `LOWER(programs.name) ASC, programs.id ASC`
	case "name_desc":
		return `LOWER(programs.name) DESC, programs.id ASC`
	case "updated_oldest":
		return `programs.updated_on ASC, programs.id ASC`
	case "updated_newest":
		return `programs.updated_on DESC, programs.id ASC`
	case "completed_first":
		return completedRank + `, LOWER(programs.name) ASC, programs.id ASC`
	default:
		return acceptingRank + `, LOWER(programs.name) ASC, programs.id ASC`
	}
}

// ListCatalog returns a paginated catalog of programs with nested skills, terms, and mentors.
func (r *ProgramRepository) ListCatalog(ctx context.Context, filter models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.ListCatalog")
	defer span.End()

	limit, offset := catalogLimitOffset(filter)
	where, args := catalogWhere(filter)

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM programs`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("count catalog programs: %w", err)
	}

	args = append(args, limit, offset)
	listQ := `SELECT` + programSelectCols + programsWithFundingFrom + where +
		fmt.Sprintf(` ORDER BY %s LIMIT $%d OFFSET $%d`, catalogOrderBy(filter.SortBy), len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list catalog programs: %w", err)
	}
	defer rows.Close()

	var programs []*models.Program
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			span.RecordError(err)
			return nil, nil, fmt.Errorf("scan catalog program: %w", err)
		}
		programs = append(programs, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("catalog rows error: %w", err)
	}

	items, err := r.attachCatalog(ctx, programs)
	if err != nil {
		span.RecordError(err)
		return nil, nil, err
	}
	return items, &models.PaginationMeta{Total: total, Limit: limit, Offset: offset}, nil
}

// GetCatalog returns one catalog item by UUID or slug.
func (r *ProgramRepository) GetCatalog(ctx context.Context, id string) (*models.ProgramCatalogItem, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.GetCatalog")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", id))

	p, err := r.GetByID(ctx, id)
	if err != nil {
		p, err = r.GetBySlug(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	items, err := r.attachCatalog(ctx, []*models.Program{p})
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	if len(items) == 0 {
		return nil, domain.ErrProgramNotFound
	}
	return items[0], nil
}

// ListCatalogMentees returns accepted/active/graduated mentees for a program.
func (r *ProgramRepository) ListCatalogMentees(ctx context.Context, programID string) ([]*models.ProgramCatalogMentee, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.ListCatalogMentees")
	defer span.End()
	span.SetAttributes(attribute.String("db.program_id", programID))

	rows, err := r.pool.Query(ctx, `
		SELECT a.user_id, u.name, u.avatar_url, u.email, up.introduction, a.status, pt.id, pt.name
		FROM applications a
		JOIN program_terms pt ON pt.id = a.program_term_id
		LEFT JOIN users u ON u.id = a.user_id
		LEFT JOIN LATERAL (
			SELECT introduction
			FROM user_profiles
			WHERE user_id = a.user_id
			  AND profile_type = 'mentee'
			ORDER BY updated_on DESC
			LIMIT 1
		) up ON true
		WHERE pt.program_id = $1
		  AND pt.status <> 'deleted'
		  AND a.role = 'mentee'
		  AND a.status IN ('accepted', 'active', 'graduated')
		ORDER BY CASE WHEN a.status = 'graduated' THEN 1 ELSE 0 END,
		         u.name NULLS LAST,
		         pt.start_date_time DESC NULLS LAST`, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list catalog mentees: %w", err)
	}
	defer rows.Close()

	var mentees []*models.ProgramCatalogMentee
	for rows.Next() {
		var m models.ProgramCatalogMentee
		if err := rows.Scan(&m.UserID, &m.Name, &m.AvatarURL, &m.Email, &m.Introduction, &m.Status, &m.TermID, &m.TermName); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan catalog mentee: %w", err)
		}
		mentees = append(mentees, &m)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("catalog mentees rows: %w", err)
	}
	if mentees == nil {
		mentees = []*models.ProgramCatalogMentee{}
	}
	return mentees, nil
}

func (r *ProgramRepository) attachCatalog(ctx context.Context, programs []*models.Program) ([]*models.ProgramCatalogItem, error) {
	items := make([]*models.ProgramCatalogItem, 0, len(programs))
	if len(programs) == 0 {
		return items, nil
	}

	ids := make([]string, len(programs))
	for i, p := range programs {
		ids[i] = p.ID
	}

	skillsBy, err := r.loadCatalogSkills(ctx, ids)
	if err != nil {
		return nil, err
	}
	termsBy, err := r.loadCatalogTerms(ctx, ids)
	if err != nil {
		return nil, err
	}
	mentorsBy, err := r.loadCatalogMentors(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, p := range programs {
		skills := skillsBy[p.ID]
		if skills == nil {
			skills = []string{}
		}
		terms := termsBy[p.ID]
		if terms == nil {
			terms = []models.ProgramCatalogTerm{}
		}
		mentors := mentorsBy[p.ID]
		if mentors == nil {
			mentors = []models.ProgramCatalogMentor{}
		}
		items = append(items, &models.ProgramCatalogItem{
			Program: *p,
			Skills:  skills,
			Terms:   terms,
			Mentors: mentors,
		})
	}
	return items, nil
}

func (r *ProgramRepository) loadCatalogSkills(ctx context.Context, ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	rows, err := r.pool.Query(ctx, `
		SELECT program_id, skill
		FROM program_skills
		WHERE program_id = ANY($1::uuid[])
		ORDER BY skill`, ids)
	if err != nil {
		return nil, fmt.Errorf("list catalog skills: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var programID, skill string
		if err := rows.Scan(&programID, &skill); err != nil {
			return nil, fmt.Errorf("scan catalog skill: %w", err)
		}
		out[programID] = append(out[programID], skill)
	}
	return out, rows.Err()
}

func (r *ProgramRepository) loadCatalogTerms(ctx context.Context, ids []string) (map[string][]models.ProgramCatalogTerm, error) {
	out := map[string][]models.ProgramCatalogTerm{}
	rows, err := r.pool.Query(ctx, `
		SELECT`+programTermCols+`
		FROM program_terms
		WHERE program_id = ANY($1::uuid[])
		  AND status <> 'deleted'
		ORDER BY start_date_time DESC NULLS LAST`, ids)
	if err != nil {
		return nil, fmt.Errorf("list catalog terms: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		t, err := scanProgramTerm(rows)
		if err != nil {
			return nil, fmt.Errorf("scan catalog term: %w", err)
		}
		out[t.ProgramID] = append(out[t.ProgramID], models.ProgramCatalogTerm{ProgramTerm: *t})
	}
	return out, rows.Err()
}

func (r *ProgramRepository) loadCatalogMentors(ctx context.Context, ids []string) (map[string][]models.ProgramCatalogMentor, error) {
	out := map[string][]models.ProgramCatalogMentor{}
	rows, err := r.pool.Query(ctx, `
		SELECT pm.id, pm.program_id, pm.user_id, u.name, u.avatar_url, up.introduction
		FROM program_members pm
		LEFT JOIN users u ON u.id = pm.user_id
		LEFT JOIN LATERAL (
			SELECT introduction
			FROM user_profiles
			WHERE user_id = pm.user_id
			  AND profile_type = 'mentor'
			ORDER BY updated_on DESC
			LIMIT 1
		) up ON true
		WHERE pm.program_id = ANY($1::uuid[])
		  AND pm.member_type = 'mentor'
		  AND pm.status = 'active'
		ORDER BY u.name NULLS LAST, pm.created_on`, ids)
	if err != nil {
		return nil, fmt.Errorf("list catalog mentors: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var programID string
		var m models.ProgramCatalogMentor
		if err := rows.Scan(&m.ID, &programID, &m.UserID, &m.Name, &m.AvatarURL, &m.Introduction); err != nil {
			return nil, fmt.Errorf("scan catalog mentor: %w", err)
		}
		out[programID] = append(out[programID], m)
	}
	return out, rows.Err()
}

// Create inserts a new program and returns the persisted record.
func (r *ProgramRepository) Create(ctx context.Context, input models.ProgramCreateInput) (*models.Program, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.Create")
	defer span.End()

	const q = `
		INSERT INTO programs (
			id, name, slug, status, is_paid, description, logo_url, website_url, repo_link,
			code_of_conduct, industry, color, lfid, cii_project_id, accept_applications,
			terms_and_conditions, mentee_needs, task_templates
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING` + programReturningCols

	p, err := scanProgram(r.pool.QueryRow(ctx, q,
		input.ID, input.Name, input.Slug, input.Status, input.IsPaid,
		input.Description, input.LogoURL, input.WebsiteURL, input.RepoLink,
		input.CodeOfConduct, input.Industry, input.Color, input.LFID, input.CIIProjectID,
		input.AcceptApplications, input.TermsAndConditions,
		nilIfEmpty(input.MenteeNeeds), nilIfEmpty(input.TaskTemplates),
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
			mentee_needs        = COALESCE($19, mentee_needs),
			task_templates      = COALESCE($20, task_templates)
		WHERE id = $1
		RETURNING id`

	var statusVal *string
	if input.Status != nil {
		s := string(*input.Status)
		statusVal = &s
	}

	var updatedID string
	err := r.pool.QueryRow(ctx, q,
		id, input.Name, input.Slug, statusVal, input.IsPaid,
		input.Description, input.LogoURL, input.WebsiteURL, input.RepoLink,
		input.CodeOfConduct, input.Industry, input.Color, input.LFID, input.CIIProjectID,
		input.AcceptApplications, input.TermsAndConditions, input.ProgramTermStatus, input.DiscoverSortRank,
		nilIfEmpty(input.MenteeNeeds), nilIfEmpty(input.TaskTemplates),
	).Scan(&updatedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProgramNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program: %w", err)
	}

	p, err := r.GetByID(ctx, updatedID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("reload updated program: %w", err)
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

// ListFundingSyncProgramIDs returns active program IDs eligible for ledger sync.
func (r *ProgramRepository) ListFundingSyncProgramIDs(ctx context.Context) ([]string, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.ListFundingSyncProgramIDs")
	defer span.End()

	const q = `
		SELECT programs.id
		FROM programs
		WHERE status NOT IN ('archived', 'draft')
		ORDER BY programs.id`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list funding sync program IDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan funding sync program ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("iterate funding sync program IDs: %w", err)
	}
	return ids, nil
}

// BulkUpsertFundingStats upserts one row per program in program_funding_stats.
func (r *ProgramRepository) BulkUpsertFundingStats(ctx context.Context, rows []models.ProgramFundingStatsUpsert) (int, error) {
	ctx, span := programTracer.Start(ctx, "db.programs.BulkUpsertFundingStats")
	defer span.End()
	span.SetAttributes(attribute.Int("db.batch_size", len(rows)))

	if len(rows) == 0 {
		return 0, nil
	}

	const q = `
		INSERT INTO program_funding_stats (program_id, amount_raised, updated_on)
		VALUES ($1, ($2::numeric / 100.0), NOW())
		ON CONFLICT (program_id) DO UPDATE
		SET amount_raised = EXCLUDED.amount_raised,
		    updated_on    = NOW()`

	batch := &pgx.Batch{}
	for i := range rows {
		batch.Queue(q, rows[i].ProgramID, rows[i].AmountRaisedCents)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := range rows {
		if _, err := br.Exec(); err != nil {
			span.RecordError(err)
			return i, fmt.Errorf("upsert program_funding_stats[%d] %s: %w", i, rows[i].ProgramID, err)
		}
	}

	return len(rows), nil
}
