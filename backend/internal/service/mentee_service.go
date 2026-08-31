// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var menteeSvcTracer = otel.Tracer("mentees-service")

// MenteeService orchestrates public mentee directory reads.
type MenteeService struct {
	repo domain.MenteeRepository
}

// NewMenteeService returns a MenteeService.
func NewMenteeService(repo domain.MenteeRepository) *MenteeService {
	return &MenteeService{repo: repo}
}

func normalizeMenteeFilter(filter models.MenteeFilter) models.MenteeFilter {
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Skill = strings.TrimSpace(filter.Skill)
	if strings.EqualFold(filter.Skill, "all") {
		filter.Skill = ""
	}
	switch strings.ToLower(strings.TrimSpace(filter.Status)) {
	case "active", "graduated":
		filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	default:
		filter.Status = ""
	}
	return filter
}

func normalizeMenteeItem(item *models.MenteeItem) {
	if item.Skills == nil {
		item.Skills = []string{}
	}
	if item.Mentors == nil {
		item.Mentors = []models.ProgramCatalogMentor{}
	}
}

func normalizeMenteeDetail(detail *models.MenteeDetail) {
	normalizeMenteeItem(&detail.MenteeItem)
	if detail.Programs == nil {
		detail.Programs = []models.MenteeProgram{}
	}
	for i := range detail.Programs {
		if detail.Programs[i].Skills == nil {
			detail.Programs[i].Skills = []string{}
		}
		if detail.Programs[i].Terms == nil {
			detail.Programs[i].Terms = []models.MenteeProgramTerm{}
		}
		if detail.Programs[i].Mentors == nil {
			detail.Programs[i].Mentors = []models.ProgramCatalogMentor{}
		}
	}
}

// List returns a paginated public mentee directory.
func (s *MenteeService) List(ctx context.Context, filter models.MenteeFilter) (*models.MenteePage, error) {
	ctx, span := menteeSvcTracer.Start(ctx, "MenteeService.List")
	defer span.End()

	page, err := s.repo.List(ctx, normalizeMenteeFilter(filter))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list mentees: %w", err)
	}
	if page.Data == nil {
		page.Data = []*models.MenteeItem{}
	}
	for _, item := range page.Data {
		normalizeMenteeItem(item)
	}
	return page, nil
}

// Summary returns unfiltered mentee and project totals.
func (s *MenteeService) Summary(ctx context.Context) (*models.MenteeSummary, error) {
	ctx, span := menteeSvcTracer.Start(ctx, "MenteeService.Summary")
	defer span.End()

	summary, err := s.repo.Summary(ctx)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get mentee summary: %w", err)
	}
	return summary, nil
}

// GetByUserID returns one public mentee profile by user ID.
func (s *MenteeService) GetByUserID(ctx context.Context, userID string) (*models.MenteeDetail, error) {
	ctx, span := menteeSvcTracer.Start(ctx, "MenteeService.GetByUserID")
	defer span.End()
	span.SetAttributes(attribute.String("mentee.user_id", userID))

	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("%w: mentee id is required", domain.ErrInvalidInput)
	}

	detail, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get mentee: %w", err)
	}
	normalizeMenteeDetail(detail)
	return detail, nil
}
