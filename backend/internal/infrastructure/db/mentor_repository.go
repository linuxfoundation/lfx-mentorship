// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var mentorTracer = otel.Tracer("mentors-db")

// mentorDisplayNameSQL is the list-card name: users.name / profile name, or the
// same fallback the frontend shows when the name is empty.
const mentorDisplayNameSQL = `COALESCE(NULLIF(TRIM(name), ''), 'Mentor')`

// MentorRepository implements domain.MentorRepository against PostgreSQL.
type MentorRepository struct {
	pool *pgxpool.Pool
}

// NewMentorRepository creates a new MentorRepository.
func NewMentorRepository(pool *pgxpool.Pool) *MentorRepository {
	return &MentorRepository{pool: pool}
}

const mentorEligibleCTE = `
	eligible AS (
		SELECT
			pm.user_id,
			pm.program_id,
			pm.created_on,
			p.name AS program_name,
			p.slug AS program_slug,
			p.description AS program_description,
			p.logo_url AS program_logo_url
		FROM program_members pm
		JOIN programs p ON p.id = pm.program_id
		WHERE pm.member_type = 'mentor'
		  AND pm.status = 'active'
		  AND p.status = 'published'
	),
	joined AS (
		SELECT user_id, MIN(created_on) AS joined_at
		FROM eligible
		GROUP BY user_id
	),
	enriched AS (
		SELECT
			j.user_id,
			j.joined_at,
			NULLIF(TRIM(COALESCE(NULLIF(TRIM(u.name), ''), CONCAT_WS(' ', up.first_name, up.last_name))), '') AS name,
			COALESCE(u.avatar_url, up.logo_url) AS avatar_url,
			up.introduction,
			COALESCE((
				SELECT ARRAY_AGG(s ORDER BY s)
				FROM jsonb_array_elements_text(
					CASE
						WHEN jsonb_typeof(up.skill_set->'skills') = 'array' THEN up.skill_set->'skills'
						ELSE '[]'::jsonb
					END
				) AS s
			), '{}') AS skills
		FROM joined j
		LEFT JOIN users u ON u.id = j.user_id
		LEFT JOIN LATERAL (
			SELECT introduction, logo_url, skill_set, first_name, last_name
			FROM user_profiles
			WHERE user_id = j.user_id
			  AND profile_type = 'mentor'
			ORDER BY updated_on DESC
			LIMIT 1
		) up ON true
	)`

func scanMentorItem(row pgx.Row) (*models.MentorItem, error) {
	var item models.MentorItem
	var joinedAt *time.Time
	var skills []string
	err := row.Scan(
		&item.UserID,
		&joinedAt,
		&item.Name,
		&item.AvatarURL,
		&item.Introduction,
		&skills,
	)
	if err != nil {
		return nil, err
	}
	if joinedAt != nil {
		item.JoinedAt = *joinedAt
	}
	if skills == nil {
		skills = []string{}
	}
	item.Skills = skills
	return &item, nil
}

func mentorListWhere(filter models.MentorFilter, args []any) (string, []any) {
	where := ` WHERE 1=1`
	if filter.Skill != "" {
		args = append(args, filter.Skill)
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM unnest(skills) AS s
			WHERE LOWER(s) = LOWER($%d)
		)`, len(args))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where += fmt.Sprintf(` AND COALESCE(name, '') ILIKE $%d`, len(args))
	}
	return where, args
}

// List returns a paginated public mentor directory.
func (r *MentorRepository) List(ctx context.Context, filter models.MentorFilter) (*models.MentorPage, error) {
	ctx, span := mentorTracer.Start(ctx, "db.mentors.List")
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
	where, args := mentorListWhere(filter, args)

	var total int
	if err := r.pool.QueryRow(ctx, `
		WITH `+mentorEligibleCTE+`
		SELECT COUNT(*) FROM enriched`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("count mentor page: %w", err)
	}

	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, `
		WITH `+mentorEligibleCTE+`
		SELECT user_id, joined_at, name, avatar_url, introduction, skills
		FROM enriched`+where+`
		ORDER BY LOWER(`+mentorDisplayNameSQL+`), `+mentorDisplayNameSQL+`, user_id
		LIMIT $`+fmt.Sprintf("%d", len(args)-1)+` OFFSET $`+fmt.Sprintf("%d", len(args)), args...)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list mentors: %w", err)
	}
	defer rows.Close()

	items := []*models.MentorItem{}
	for rows.Next() {
		item, err := scanMentorItem(rows)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan mentor: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("mentor rows: %w", err)
	}

	return &models.MentorPage{
		Data: items,
		Meta: models.PaginationMeta{Total: total, Limit: limit, Offset: offset},
	}, nil
}

// Summary returns unfiltered mentor and program totals for the directory header.
func (r *MentorRepository) Summary(ctx context.Context) (*models.MentorSummary, error) {
	ctx, span := mentorTracer.Start(ctx, "db.mentors.Summary")
	defer span.End()

	var mentorCount, programCount int
	if err := r.pool.QueryRow(ctx, `
		WITH `+mentorEligibleCTE+`
		SELECT
			(SELECT COUNT(*) FROM joined),
			(SELECT COUNT(DISTINCT program_id) FROM eligible)`).Scan(&mentorCount, &programCount); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("count mentor summary: %w", err)
	}
	return &models.MentorSummary{MentorCount: mentorCount, ProgramCount: programCount}, nil
}

// GetByUserID returns one public mentor profile by user ID.
func (r *MentorRepository) GetByUserID(ctx context.Context, userID string) (*models.MentorDetail, error) {
	ctx, span := mentorTracer.Start(ctx, "db.mentors.GetByUserID")
	defer span.End()
	span.SetAttributes(attribute.String("db.user_id", userID))

	item, err := scanMentorItem(r.pool.QueryRow(ctx, `
		WITH `+mentorEligibleCTE+`
		SELECT user_id, joined_at, name, avatar_url, introduction, skills
		FROM enriched
		WHERE user_id = $1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrMentorNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get mentor: %w", err)
	}

	var githubURL, linkedInURL *string
	if err := r.pool.QueryRow(ctx, `
		SELECT
			NULLIF(TRIM(profile_links->>'githubProfileLink'), ''),
			NULLIF(TRIM(profile_links->>'linkedinProfileLink'), '')
		FROM user_profiles
		WHERE user_id = $1
		  AND profile_type = 'mentor'
		ORDER BY updated_on DESC
		LIMIT 1`, userID).Scan(&githubURL, &linkedInURL); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		span.RecordError(err)
		return nil, fmt.Errorf("get mentor profile links: %w", err)
	}

	programs, err := r.loadMentorPrograms(ctx, userID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	current, graduated, err := r.loadMentorMentees(ctx, userID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return &models.MentorDetail{
		MentorItem:       *item,
		GithubURL:        githubURL,
		LinkedInURL:      linkedInURL,
		Programs:         programs,
		CurrentMentees:   current,
		GraduatedMentees: graduated,
		Stats: models.MentorStats{
			ProgramsMentoring: len(programs),
			CurrentMentees:    len(current),
			MenteesGraduated:  len(graduated),
		},
	}, nil
}

func (r *MentorRepository) loadMentorPrograms(ctx context.Context, userID string) ([]models.MentorProgram, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.name, p.slug, p.description, p.logo_url
		FROM program_members pm
		JOIN programs p ON p.id = pm.program_id
		WHERE pm.user_id = $1
		  AND pm.member_type = 'mentor'
		  AND pm.status = 'active'
		  AND p.status = 'published'
		ORDER BY p.name, p.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list mentor programs: %w", err)
	}
	defer rows.Close()

	programs := []models.MentorProgram{}
	ids := []string{}
	for rows.Next() {
		var program models.MentorProgram
		if err := rows.Scan(&program.ID, &program.Name, &program.Slug, &program.Description, &program.LogoURL); err != nil {
			return nil, fmt.Errorf("scan mentor program: %w", err)
		}
		program.Skills = []string{}
		program.Terms = []models.MentorProgramTerm{}
		program.Mentors = []models.ProgramCatalogMentor{}
		programs = append(programs, program)
		ids = append(ids, program.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mentor program rows: %w", err)
	}

	termsBy, err := r.loadProgramTerms(ctx, ids)
	if err != nil {
		return nil, err
	}
	skillsBy, err := r.loadProgramSkills(ctx, ids)
	if err != nil {
		return nil, err
	}
	mentorsBy, err := r.loadMentorsByProgram(ctx, ids)
	if err != nil {
		return nil, err
	}

	for i := range programs {
		id := programs[i].ID
		if terms := termsBy[id]; terms != nil {
			programs[i].Terms = terms
		}
		if skills := skillsBy[id]; skills != nil {
			programs[i].Skills = skills
		}
		if mentors := mentorsBy[id]; mentors != nil {
			programs[i].Mentors = mentors
		}
	}
	return programs, nil
}

func (r *MentorRepository) loadProgramTerms(ctx context.Context, ids []string) (map[string][]models.MentorProgramTerm, error) {
	out := map[string][]models.MentorProgramTerm{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, program_id, name, status, start_date_time, end_date_time,
			application_start_date, application_end_date
		FROM program_terms
		WHERE program_id = ANY($1::uuid[])
		  AND status <> 'deleted'
		ORDER BY start_date_time DESC NULLS LAST, name`, ids)
	if err != nil {
		return nil, fmt.Errorf("list mentor program terms: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var programID string
		var term models.MentorProgramTerm
		if err := rows.Scan(
			&term.ID, &programID, &term.Name, &term.Status,
			&term.StartDateTime, &term.EndDateTime,
			&term.ApplicationStartDate, &term.ApplicationEndDate,
		); err != nil {
			return nil, fmt.Errorf("scan mentor program term: %w", err)
		}
		out[programID] = append(out[programID], term)
	}
	return out, rows.Err()
}

func (r *MentorRepository) loadProgramSkills(ctx context.Context, ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT program_id, skill
		FROM program_skills
		WHERE program_id = ANY($1::uuid[])
		ORDER BY skill`, ids)
	if err != nil {
		return nil, fmt.Errorf("list mentor program skills: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var programID, skill string
		if err := rows.Scan(&programID, &skill); err != nil {
			return nil, fmt.Errorf("scan mentor program skill: %w", err)
		}
		out[programID] = append(out[programID], skill)
	}
	return out, rows.Err()
}

func (r *MentorRepository) loadMentorsByProgram(ctx context.Context, ids []string) (map[string][]models.ProgramCatalogMentor, error) {
	out := map[string][]models.ProgramCatalogMentor{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT pm.id, pm.program_id, pm.user_id, u.name, u.avatar_url
		FROM program_members pm
		LEFT JOIN users u ON u.id = pm.user_id
		WHERE pm.program_id = ANY($1::uuid[])
		  AND pm.member_type = 'mentor'
		  AND pm.status = 'active'
		ORDER BY u.name NULLS LAST, pm.created_on`, ids)
	if err != nil {
		return nil, fmt.Errorf("list mentor program mentors: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var programID string
		var m models.ProgramCatalogMentor
		if err := rows.Scan(&m.ID, &programID, &m.UserID, &m.Name, &m.AvatarURL); err != nil {
			return nil, fmt.Errorf("scan mentor program mentor: %w", err)
		}
		out[programID] = append(out[programID], m)
	}
	return out, rows.Err()
}

func (r *MentorRepository) loadMentorMentees(ctx context.Context, mentorUserID string) ([]models.MentorMentee, []models.MentorMentee, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (a.user_id, CASE WHEN a.status = 'graduated' THEN 1 ELSE 0 END)
			a.user_id, a.status, p.name, pt.name,
			NULLIF(TRIM(COALESCE(NULLIF(TRIM(u.name), ''), CONCAT_WS(' ', up.first_name, up.last_name))), ''),
			COALESCE(u.avatar_url, up.logo_url),
			up.introduction
		FROM applications a
		JOIN program_terms pt ON pt.id = a.program_term_id
		JOIN programs p ON p.id = pt.program_id
		JOIN program_members pm ON pm.program_id = p.id
			AND pm.user_id = $1
			AND pm.member_type = 'mentor'
			AND pm.status = 'active'
		LEFT JOIN users u ON u.id = a.user_id
		LEFT JOIN LATERAL (
			SELECT introduction, logo_url, first_name, last_name
			FROM user_profiles
			WHERE user_id = a.user_id
			  AND profile_type = 'mentee'
			ORDER BY updated_on DESC
			LIMIT 1
		) up ON true
		WHERE a.role = 'mentee'
		  AND a.status IN ('accepted', 'active', 'graduated')
		  AND pt.status <> 'deleted'
		  AND p.status = 'published'
		ORDER BY a.user_id,
			CASE WHEN a.status = 'graduated' THEN 1 ELSE 0 END,
			pt.start_date_time DESC NULLS LAST,
			a.created_on DESC`, mentorUserID)
	if err != nil {
		return nil, nil, fmt.Errorf("list mentor mentees: %w", err)
	}
	defer rows.Close()

	current := []models.MentorMentee{}
	graduated := []models.MentorMentee{}
	for rows.Next() {
		var m models.MentorMentee
		if err := rows.Scan(
			&m.UserID, &m.Status, &m.ProgramName, &m.TermName,
			&m.Name, &m.AvatarURL, &m.Introduction,
		); err != nil {
			return nil, nil, fmt.Errorf("scan mentor mentee: %w", err)
		}
		if m.Status == models.MenteeStatusGraduated {
			graduated = append(graduated, m)
			continue
		}
		current = append(current, m)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("mentor mentee rows: %w", err)
	}
	return current, graduated, nil
}
