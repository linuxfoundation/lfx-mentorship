// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
)

var platformSummaryTracer = otel.Tracer("platform-summary-db")

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
		SELECT DISTINCT a.user_id
		FROM applications a
		JOIN program_terms pt ON pt.id = a.program_term_id
		JOIN published_programs p ON p.id = pt.program_id
		WHERE a.role   = 'mentee'
		  AND a.status = 'graduated'
		  AND pt.status <> 'deleted'
	)
SELECT
	(SELECT COUNT(*) FROM published_programs),
	(SELECT COUNT(*) FROM accepting_programs),
	(SELECT COUNT(*) FROM mentor_users),
	(SELECT COUNT(*) FROM graduated_mentee_users)`

// Summary returns the aggregated landing/marketing counts.
func (r *PlatformSummaryRepository) Summary(ctx context.Context) (*models.PlatformSummary, error) {
	ctx, span := platformSummaryTracer.Start(ctx, "db.platform_summary.Summary")
	defer span.End()

	var s models.PlatformSummary
	if err := r.pool.QueryRow(ctx, platformSummaryQuery).Scan(
		&s.ProgramCount,
		&s.AcceptingProgramCount,
		&s.MentorCount,
		&s.GraduatedMenteeCount,
	); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("query platform summary: %w", err)
	}
	return &s, nil
}
