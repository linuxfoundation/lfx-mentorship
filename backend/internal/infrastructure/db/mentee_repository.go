// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var menteeTracer = otel.Tracer("mentees-db")

// menteeDisplayNameSQL is the list-card name: users.name / profile name, or the
// same fallback the frontend shows when the name is empty.
const menteeDisplayNameSQL = `COALESCE(NULLIF(TRIM(name), ''), 'Mentee')`

// MenteeRepository implements domain.MenteeRepository against PostgreSQL.
type MenteeRepository struct {
	pool *pgxpool.Pool
}

// NewMenteeRepository creates a new MenteeRepository.
func NewMenteeRepository(pool *pgxpool.Pool) *MenteeRepository {
	return &MenteeRepository{pool: pool}
}

// menteeProfileStatuses are the application statuses shown on a public mentee
// profile, in display precedence: when a mentee holds several, the earliest in
// the list is the one featured in the header and on the program card.
var menteeProfileStatuses = []string{
	string(models.MenteeStatusAccepted),
	string(models.MenteeStatusGraduated),
	string(models.MenteeStatusPending),
	string(models.MenteeStatusDeclined),
	string(models.MenteeStatusWithdrawn),
}

// menteeDirectoryStatuses are the statuses that list a mentee in the directory
// and its counts: only accepted and graduated mentees are publicly listed.
var menteeDirectoryStatuses = menteeProfileStatuses[:2]

// menteeStatusPrecedence ranks a status by its position in menteeProfileStatuses;
// a lower rank wins.
func menteeStatusPrecedence(status models.MenteeStatus) int {
	return slices.Index(menteeProfileStatuses, string(status))
}

// menteeEligibleCTE takes the visible application statuses, in precedence
// order, as $1.
const menteeEligibleCTE = `
	eligible AS (
		SELECT
			a.user_id,
			a.status AS application_status,
			a.created_on,
			pt.id AS term_id,
			pt.name AS term_name,
			pt.start_date_time,
			pt.end_date_time,
			p.id AS program_id,
			p.name AS program_name,
			p.slug AS program_slug,
			p.description AS program_description,
			p.logo_url AS program_logo_url
		FROM applications a
		JOIN program_terms pt ON pt.id = a.program_term_id
		JOIN programs p ON p.id = pt.program_id
		WHERE a.role = 'mentee'
		  AND a.status = ANY($1::text[])
		  AND pt.status <> 'deleted'
		  AND p.status = 'published'
	),
	featured AS (
		SELECT DISTINCT ON (e.user_id)
			e.*,
			j.joined_at
		FROM eligible e
		JOIN (
			-- Dated from the first accepted or graduated application when there is
			-- one, so a profile and its directory card agree.
			SELECT user_id, COALESCE(
				MIN(created_on) FILTER (WHERE application_status IN ('accepted', 'graduated')),
				MIN(created_on)
			) AS joined_at
			FROM eligible
			GROUP BY user_id
		) j ON j.user_id = e.user_id
		ORDER BY e.user_id,
			array_position($1::text[], e.application_status::text),
			e.start_date_time DESC NULLS LAST,
			e.created_on DESC
	),
	enriched AS (
		SELECT
			f.user_id,
			f.application_status,
			f.joined_at,
			f.program_id,
			f.program_name,
			f.program_slug,
			f.program_logo_url,
			NULLIF(TRIM(COALESCE(NULLIF(TRIM(u.name), ''), CONCAT_WS(' ', up.first_name, up.last_name))), '') AS name,
			COALESCE(u.avatar_url, up.logo_url) AS avatar_url,
			up.introduction,
			COALESCE((
				SELECT ARRAY_AGG(e #>> '{}' ORDER BY e #>> '{}')
				FROM jsonb_array_elements(
					CASE
						WHEN jsonb_typeof(up.skill_set->'skills') = 'array' THEN up.skill_set->'skills'
						ELSE '[]'::jsonb
					END
				) AS e
				-- A null element would scan as NULL into []string and fail the whole page.
				WHERE jsonb_typeof(e) = 'string'
			), '{}') AS skills
		FROM featured f
		LEFT JOIN users u ON u.id = f.user_id
		LEFT JOIN LATERAL (
			SELECT introduction, logo_url, skill_set, first_name, last_name, created_on
			FROM user_profiles
			WHERE user_id = f.user_id
			  AND profile_type = 'mentee'
			ORDER BY updated_on DESC
			LIMIT 1
		) up ON true
	)`

func scanMenteeItem(row pgx.Row) (*models.MenteeItem, error) {
	var item models.MenteeItem
	var (
		status                   *string
		joinedAt                 *time.Time
		programID, programName   *string
		programSlug, programLogo *string
		skills                   []string
	)
	err := row.Scan(
		&item.UserID,
		&status,
		&joinedAt,
		&programID,
		&programName,
		&programSlug,
		&programLogo,
		&item.Name,
		&item.AvatarURL,
		&item.Introduction,
		&skills,
	)
	if err != nil {
		return nil, err
	}
	if status != nil {
		item.Status = models.MenteeStatus(*status)
	}
	if joinedAt != nil {
		item.JoinedAt = *joinedAt
	}
	if programID != nil && programName != nil && programSlug != nil {
		item.Program = &models.MenteeProgramRef{
			ID:      *programID,
			Name:    *programName,
			Slug:    *programSlug,
			LogoURL: programLogo,
		}
	}
	if skills == nil {
		skills = []string{}
	}
	item.Skills = skills
	item.Mentors = []models.ProgramCatalogMentor{}
	return &item, nil
}

func menteeListWhere(filter models.MenteeFilter, args []any) (string, []any) {
	where := ` WHERE 1=1`
	switch strings.ToLower(strings.TrimSpace(filter.Status)) {
	case "active":
		where += ` AND application_status = 'accepted'`
	case "graduated":
		where += ` AND application_status = 'graduated'`
	}
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

// List returns a paginated public mentee directory.
func (r *MenteeRepository) List(ctx context.Context, filter models.MenteeFilter) (*models.MenteePage, error) {
	ctx, span := menteeTracer.Start(ctx, "db.mentees.List")
	defer span.End()

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{menteeDirectoryStatuses}
	where, args := menteeListWhere(filter, args)

	var total int
	if err := r.pool.QueryRow(ctx, `
		WITH `+menteeEligibleCTE+`
		SELECT COUNT(*) FROM enriched`+where, args...).Scan(&total); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("count mentee page: %w", err)
	}

	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, `
		WITH `+menteeEligibleCTE+`
		SELECT user_id, application_status, joined_at, program_id, program_name, program_slug,
		       program_logo_url, name, avatar_url, introduction, skills
		FROM enriched`+where+`
		ORDER BY LOWER(`+menteeDisplayNameSQL+`), `+menteeDisplayNameSQL+`, user_id
		LIMIT $`+fmt.Sprintf("%d", len(args)-1)+` OFFSET $`+fmt.Sprintf("%d", len(args)), args...)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list mentees: %w", err)
	}
	defer rows.Close()

	items := []*models.MenteeItem{}
	for rows.Next() {
		item, err := scanMenteeItem(rows)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("scan mentee: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("mentee rows: %w", err)
	}

	if err := r.attachFeaturedMentors(ctx, items); err != nil {
		span.RecordError(err)
		return nil, err
	}

	return &models.MenteePage{
		Data: items,
		Meta: models.PaginationMeta{Total: total, Limit: limit, Offset: offset},
	}, nil
}

// Summary returns unfiltered mentee and program totals for the directory header.
func (r *MenteeRepository) Summary(ctx context.Context) (*models.MenteeSummary, error) {
	ctx, span := menteeTracer.Start(ctx, "db.mentees.Summary")
	defer span.End()

	var menteeCount, programCount int
	if err := r.pool.QueryRow(ctx, `
		WITH `+menteeEligibleCTE+`
		SELECT
			(SELECT COUNT(*) FROM featured),
			(SELECT COUNT(DISTINCT program_id) FROM eligible)`, menteeDirectoryStatuses).Scan(&menteeCount, &programCount); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("count mentee summary: %w", err)
	}
	return &models.MenteeSummary{MenteeCount: menteeCount, ProgramCount: programCount}, nil
}

// GetByUserID returns one public mentee profile by user ID. Unlike the
// directory, it includes mentees whose applications are pending, declined, or
// withdrawn.
func (r *MenteeRepository) GetByUserID(ctx context.Context, userID string) (*models.MenteeDetail, error) {
	ctx, span := menteeTracer.Start(ctx, "db.mentees.GetByUserID")
	defer span.End()
	span.SetAttributes(attribute.String("db.user_id", userID))

	item, err := scanMenteeItem(r.pool.QueryRow(ctx, `
		WITH `+menteeEligibleCTE+`
		SELECT user_id, application_status, joined_at, program_id, program_name, program_slug,
		       program_logo_url, name, avatar_url, introduction, skills
		FROM enriched
		WHERE user_id = $2`, menteeProfileStatuses, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrMenteeNotFound
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get mentee: %w", err)
	}

	var githubURL, linkedInURL *string
	if err := r.pool.QueryRow(ctx, `
		SELECT
			NULLIF(TRIM(profile_links->>'githubProfileLink'), ''),
			NULLIF(TRIM(profile_links->>'linkedinProfileLink'), '')
		FROM user_profiles
		WHERE user_id = $1
		  AND profile_type = 'mentee'
		ORDER BY updated_on DESC
		LIMIT 1`, userID).Scan(&githubURL, &linkedInURL); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		span.RecordError(err)
		return nil, fmt.Errorf("get mentee profile links: %w", err)
	}

	if err := r.attachFeaturedMentors(ctx, []*models.MenteeItem{item}); err != nil {
		span.RecordError(err)
		return nil, err
	}

	programs, err := r.loadMenteePrograms(ctx, userID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return &models.MenteeDetail{
		MenteeItem:  *item,
		GithubURL:   githubURL,
		LinkedInURL: linkedInURL,
		Programs:    programs,
	}, nil
}

func (r *MenteeRepository) attachFeaturedMentors(ctx context.Context, items []*models.MenteeItem) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.Program == nil || item.Program.ID == "" {
			continue
		}
		if _, ok := seen[item.Program.ID]; ok {
			continue
		}
		seen[item.Program.ID] = struct{}{}
		ids = append(ids, item.Program.ID)
	}
	mentorsBy, err := r.loadMentorsByProgram(ctx, ids)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Program == nil {
			item.Mentors = []models.ProgramCatalogMentor{}
			continue
		}
		mentors := mentorsBy[item.Program.ID]
		if mentors == nil {
			mentors = []models.ProgramCatalogMentor{}
		}
		item.Mentors = mentors
	}
	return nil
}

func (r *MenteeRepository) loadMenteePrograms(ctx context.Context, userID string) ([]models.MenteeProgram, error) {
	// One row per term: a withdrawn application is kept beside its
	// reapplication, and the reapplication is the one to show.
	rows, err := r.pool.Query(ctx, `
		SELECT program_id, program_name, slug, description, logo_url,
		       term_id, term_name, start_date_time, end_date_time, status
		FROM (
			SELECT DISTINCT ON (pt.id)
				p.id AS program_id, p.name AS program_name, p.slug, p.description, p.logo_url,
				pt.id AS term_id, pt.name AS term_name, pt.start_date_time, pt.end_date_time, a.status
			FROM applications a
			JOIN program_terms pt ON pt.id = a.program_term_id
			JOIN programs p ON p.id = pt.program_id
			WHERE a.user_id = $1
			  AND a.role = 'mentee'
			  AND a.status = ANY($2::text[])
			  AND pt.status <> 'deleted'
			  AND p.status = 'published'
			ORDER BY pt.id, array_position($2::text[], a.status::text), a.created_on DESC
		) terms
		ORDER BY program_name, start_date_time DESC NULLS LAST`, userID, menteeProfileStatuses)
	if err != nil {
		return nil, fmt.Errorf("list mentee programs: %w", err)
	}
	defer rows.Close()

	byID := map[string]*models.MenteeProgram{}
	order := []string{}
	for rows.Next() {
		var (
			program models.MenteeProgram
			term    models.MenteeProgramTerm
		)
		if err := rows.Scan(
			&program.ID, &program.Name, &program.Slug, &program.Description, &program.LogoURL,
			&term.ID, &term.Name, &term.StartDateTime, &term.EndDateTime, &term.ApplicationStatus,
		); err != nil {
			return nil, fmt.Errorf("scan mentee program: %w", err)
		}
		existing, ok := byID[program.ID]
		if !ok {
			program.Skills = []string{}
			program.Terms = []models.MenteeProgramTerm{}
			program.Mentors = []models.ProgramCatalogMentor{}
			program.Status = term.ApplicationStatus
			byID[program.ID] = &program
			existing = &program
			order = append(order, program.ID)
		}
		existing.Terms = append(existing.Terms, term)
		if menteeStatusPrecedence(term.ApplicationStatus) < menteeStatusPrecedence(existing.Status) {
			existing.Status = term.ApplicationStatus
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mentee program rows: %w", err)
	}

	ids := append([]string{}, order...)
	skillsBy, err := r.loadProgramSkills(ctx, ids)
	if err != nil {
		return nil, err
	}
	mentorsBy, err := r.loadMentorsByProgram(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]models.MenteeProgram, 0, len(order))
	for _, id := range order {
		program := byID[id]
		if skills := skillsBy[id]; skills != nil {
			program.Skills = skills
		}
		if mentors := mentorsBy[id]; mentors != nil {
			program.Mentors = mentors
		}
		out = append(out, *program)
	}
	return out, nil
}

func (r *MenteeRepository) loadProgramSkills(ctx context.Context, ids []string) (map[string][]string, error) {
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
		return nil, fmt.Errorf("list mentee program skills: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var programID, skill string
		if err := rows.Scan(&programID, &skill); err != nil {
			return nil, fmt.Errorf("scan mentee program skill: %w", err)
		}
		out[programID] = append(out[programID], skill)
	}
	return out, rows.Err()
}

func (r *MenteeRepository) loadMentorsByProgram(ctx context.Context, ids []string) (map[string][]models.ProgramCatalogMentor, error) {
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
		return nil, fmt.Errorf("list mentee program mentors: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var programID string
		var m models.ProgramCatalogMentor
		if err := rows.Scan(&m.ID, &programID, &m.UserID, &m.Name, &m.AvatarURL); err != nil {
			return nil, fmt.Errorf("scan mentee program mentor: %w", err)
		}
		out[programID] = append(out[programID], m)
	}
	return out, rows.Err()
}
