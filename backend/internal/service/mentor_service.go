// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var mentorSvcTracer = otel.Tracer("mentors-service")

// MentorService orchestrates public mentor directory reads.
type MentorService struct {
	repo domain.MentorRepository
}

// NewMentorService returns a MentorService.
func NewMentorService(repo domain.MentorRepository) *MentorService {
	return &MentorService{repo: repo}
}

func normalizeMentorFilter(filter models.MentorFilter) models.MentorFilter {
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Skill = strings.TrimSpace(filter.Skill)
	if strings.EqualFold(filter.Skill, "all") {
		filter.Skill = ""
	}
	return filter
}

func normalizeMentorItem(item *models.MentorItem) {
	if item.Skills == nil {
		item.Skills = []string{}
	}
}

func normalizeMentorDetail(detail *models.MentorDetail) {
	normalizeMentorItem(&detail.MentorItem)
	if detail.Programs == nil {
		detail.Programs = []models.MentorProgram{}
	}
	if detail.CurrentMentees == nil {
		detail.CurrentMentees = []models.MentorMentee{}
	}
	if detail.GraduatedMentees == nil {
		detail.GraduatedMentees = []models.MentorMentee{}
	}
	for i := range detail.Programs {
		if detail.Programs[i].Skills == nil {
			detail.Programs[i].Skills = []string{}
		}
		if detail.Programs[i].Terms == nil {
			detail.Programs[i].Terms = []models.MentorProgramTerm{}
		}
		if detail.Programs[i].Mentors == nil {
			detail.Programs[i].Mentors = []models.ProgramCatalogMentor{}
		}
	}
}

// List returns a paginated public mentor directory.
func (s *MentorService) List(ctx context.Context, filter models.MentorFilter) (*models.MentorPage, error) {
	ctx, span := mentorSvcTracer.Start(ctx, "MentorService.List")
	defer span.End()

	page, err := s.repo.List(ctx, normalizeMentorFilter(filter))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list mentors: %w", err)
	}
	if page.Data == nil {
		page.Data = []*models.MentorItem{}
	}
	for _, item := range page.Data {
		normalizeMentorItem(item)
	}
	return page, nil
}

// Summary returns unfiltered mentor and program totals.
func (s *MentorService) Summary(ctx context.Context) (*models.MentorSummary, error) {
	ctx, span := mentorSvcTracer.Start(ctx, "MentorService.Summary")
	defer span.End()

	summary, err := s.repo.Summary(ctx)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get mentor summary: %w", err)
	}
	return summary, nil
}

// GetByUserID returns one public mentor profile by user ID.
func (s *MentorService) GetByUserID(ctx context.Context, userID string) (*models.MentorDetail, error) {
	ctx, span := mentorSvcTracer.Start(ctx, "MentorService.GetByUserID")
	defer span.End()
	span.SetAttributes(attribute.String("mentor.user_id", userID))

	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: mentor id is required", domain.ErrInvalidInput)
	}
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("%w: mentor id must be a UUID", domain.ErrInvalidInput)
	}

	detail, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get mentor: %w", err)
	}
	normalizeMentorDetail(detail)
	return detail, nil
}
