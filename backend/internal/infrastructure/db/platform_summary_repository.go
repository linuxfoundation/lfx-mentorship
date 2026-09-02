// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
)

var platformSummaryTracer = otel.Tracer("platform-summary-db")

// platformSummaryMenteePreviewLimit is how many graduated mentees the
// landing hero preview includes.
const platformSummaryMenteePreviewLimit = 4

// PlatformSummaryRepository implements domain.PlatformSummaryRepository against PostgreSQL.
type PlatformSummaryRepository struct {
	pool *pgxpool.Pool
}

// NewPlatformSummaryRepository creates a new PlatformSummaryRepository.
func NewPlatformSummaryRepository(pool *pgxpool.Pool) *PlatformSummaryRepository {
	return &PlatformSummaryRepository{pool: pool}
}

// platformSummaryQuery aggregates all landing counts in a single round trip.
// Each metric is scoped to published programs so the totals track what the
// public directory pages already surface.
const platformSummaryQuery = `
WITH
	published_programs AS (
		SELECT id
		FROM programs
		WHERE status = 'published'
	),
	accepting_programs AS (
		SELECT DISTINCT pt.program_id
		FROM program_terms pt
		JOIN published_programs p ON p.id = pt.program_id
		WHERE pt.status = 'open'
		  AND pt.application_start_date IS NOT NULL
		  AND pt.application_end_date   IS NOT NULL
		  AND NOW() BETWEEN pt.application_start_date AND pt.application_end_date
	),
	mentor_users AS (
		SELECT DISTINCT pm.user_id
		FROM program_members pm
		JOIN published_programs p ON p.id = pm.program_id
		WHERE pm.member_type = 'mentor'
		  AND pm.status      = 'active'
	),
	graduated_mentee_users AS (
		SELECT DISTINCT ON (a.user_id)
			a.user_id,
			a.updated_on AS graduated_on
		FROM applications a
		JOIN program_terms pt ON pt.id = a.program_term_id
		JOIN published_programs p ON p.id = pt.program_id
		WHERE a.role   = 'mentee'
		  AND a.status = 'graduated'
		  AND pt.status <> 'deleted'
		ORDER BY a.user_id, a.updated_on DESC
	),
	graduated_mentee_users_data AS (
		SELECT
			gmu.user_id,
			NULLIF(TRIM(COALESCE(NULLIF(TRIM(u.name), ''), CONCAT_WS(' ', up.first_name, up.last_name))), '') AS name,
			COALESCE(u.avatar_url, up.logo_url) AS avatar_url,
			gmu.graduated_on
		FROM graduated_mentee_users gmu
		LEFT JOIN users u ON u.id = gmu.user_id
		LEFT JOIN LATERAL (
			SELECT logo_url, first_name, last_name
			FROM user_profiles
			WHERE user_id = gmu.user_id
			  AND profile_type = 'mentee'
			ORDER BY updated_on DESC
			LIMIT 1
		) up ON true
	)
SELECT
	(SELECT COUNT(*) FROM published_programs),
	(SELECT COUNT(*) FROM accepting_programs),
	(SELECT COUNT(*) FROM mentor_users),
	(SELECT COUNT(*) FROM graduated_mentee_users),
	COALESCE((
		SELECT json_agg(json_build_object(
			'name', preview.name,
			'avatar_url', preview.avatar_url
		) ORDER BY preview.graduated_on DESC NULLS LAST)
		FROM (
			SELECT name, avatar_url, graduated_on
			FROM graduated_mentee_users_data
			ORDER BY graduated_on DESC NULLS LAST
			LIMIT $1
		) preview
	), '[]'::json)`

// Summary returns the aggregated landing/marketing counts.
func (r *PlatformSummaryRepository) Summary(ctx context.Context) (*models.PlatformSummary, error) {
	ctx, span := platformSummaryTracer.Start(ctx, "db.platform_summary.Summary")
	defer span.End()

	var (
		s       models.PlatformSummary
		preview []byte
	)
	if err := r.pool.QueryRow(ctx, platformSummaryQuery, platformSummaryMenteePreviewLimit).Scan(
		&s.ProgramCount,
		&s.AcceptingProgramCount,
		&s.MentorCount,
		&s.GraduatedMenteeCount,
		&preview,
	); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("query platform summary: %w", err)
	}
	if err := json.Unmarshal(preview, &s.GraduatedMenteeUsers); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("decode graduated mentee preview: %w", err)
	}
	if s.GraduatedMenteeUsers == nil {
		s.GraduatedMenteeUsers = []models.PlatformSummaryMentee{}
	}
	return &s, nil
}
