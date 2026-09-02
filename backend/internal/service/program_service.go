// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var programSvcTracer = otel.Tracer("programs-service")

// ProgramService orchestrates program reads and writes.
type ProgramService struct {
	repo     domain.ProgramRepository
	termRepo domain.ProgramTermRepository
	appRepo  domain.ApplicationRepository
}

// NewProgramService returns a ProgramService.
func NewProgramService(repo domain.ProgramRepository, termRepo domain.ProgramTermRepository, appRepo domain.ApplicationRepository) *ProgramService {
	return &ProgramService{repo: repo, termRepo: termRepo, appRepo: appRepo}
}

// programTransitions defines the valid next states for each program status.
var programTransitions = map[models.ProgramStatus][]models.ProgramStatus{
	models.ProgramStatusDraft:     {models.ProgramStatusSubmitted},
	models.ProgramStatusSubmitted: {models.ProgramStatusPublished, models.ProgramStatusRejected},
	models.ProgramStatusPublished: {models.ProgramStatusArchived, models.ProgramStatusHidden},
	models.ProgramStatusRejected:  {models.ProgramStatusSubmitted}, // resubmit directly; no detour through draft
	models.ProgramStatusArchived:  {},
	models.ProgramStatusHidden:    {models.ProgramStatusPublished, models.ProgramStatusArchived},
}

// GetByID returns the program with the given ID.
func (s *ProgramService) GetByID(ctx context.Context, id string) (*models.Program, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", id))

	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program: %w", err)
	}
	return p, nil
}

// GetBySlug returns the program with the given slug.
func (s *ProgramService) GetBySlug(ctx context.Context, slug string) (*models.Program, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.GetBySlug")
	defer span.End()
	span.SetAttributes(attribute.String("program.slug", slug))

	p, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program by slug: %w", err)
	}
	return p, nil
}

// List returns a paginated list of programs.
func (s *ProgramService) List(ctx context.Context, filter models.ProgramFilter) ([]*models.Program, *models.PaginationMeta, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.List")
	defer span.End()

	programs, meta, err := s.repo.List(ctx, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list programs: %w", err)
	}
	return programs, meta, nil
}

func applyCatalogLabels(items []*models.ProgramCatalogItem, now time.Time) {
	for _, item := range items {
		if item.Skills == nil {
			item.Skills = []string{}
		}
		if item.Terms == nil {
			item.Terms = []models.ProgramCatalogTerm{}
		}
		if item.Mentors == nil {
			item.Mentors = []models.ProgramCatalogMentor{}
		}
		for i := range item.Terms {
			item.Terms[i].DiscoveryLabel = item.Terms[i].ProgramTerm.DiscoveryLabel(now)
		}
	}
}

// ListCatalog returns a paginated public catalog of programs with nested skills, terms, and mentors.
func (s *ProgramService) ListCatalog(ctx context.Context, filter models.ProgramFilter) ([]*models.ProgramCatalogItem, *models.PaginationMeta, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.ListCatalog")
	defer span.End()

	filter = normalizeCatalogFilter(filter)

	items, meta, err := s.repo.ListCatalog(ctx, filter)
	if err != nil {
		span.RecordError(err)
		return nil, nil, fmt.Errorf("list program catalog: %w", err)
	}
	applyCatalogLabels(items, time.Now())
	return items, meta, nil
}

func normalizeCatalogFilter(filter models.ProgramFilter) models.ProgramFilter {
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Skill = strings.TrimSpace(filter.Skill)
	if strings.EqualFold(filter.Skill, "all") {
		filter.Skill = ""
	}
	// Public catalog is published programs only.
	filter.Status = string(models.ProgramStatusPublished)

	switch strings.ToLower(strings.TrimSpace(filter.DiscoveryStatus)) {
	case "acceptance", "in-progress", "completed":
		filter.DiscoveryStatus = strings.ToLower(strings.TrimSpace(filter.DiscoveryStatus))
	default:
		filter.DiscoveryStatus = ""
	}

	switch filter.SortBy {
	case "accepting_first", "completed_first", "name_asc", "name_desc", "updated_oldest", "updated_newest":
	default:
		filter.SortBy = "accepting_first"
	}
	return filter
}

// GetCatalog returns one catalog item by UUID or slug.
func (s *ProgramService) GetCatalog(ctx context.Context, id string) (*models.ProgramCatalogItem, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.GetCatalog")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", id))

	item, err := s.repo.GetCatalog(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get program catalog: %w", err)
	}
	applyCatalogLabels([]*models.ProgramCatalogItem{item}, time.Now())
	return item, nil
}

// ListCatalogMentees returns accepted/active/graduated mentees for a program.
func (s *ProgramService) ListCatalogMentees(ctx context.Context, programID string) ([]*models.ProgramCatalogMentee, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.ListCatalogMentees")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", programID))

	mentees, err := s.repo.ListCatalogMentees(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list catalog mentees: %w", err)
	}
	return mentees, nil
}

// Create validates input and creates a program.
func (s *ProgramService) Create(ctx context.Context, input models.ProgramCreateInput) (*models.Program, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.Create")
	defer span.End()

	if strings.TrimSpace(input.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(input.Slug) == "" {
		return nil, fmt.Errorf("%w: slug is required", domain.ErrInvalidInput)
	}
	input.Status = models.ProgramStatusDraft // programs always start as draft
	input.ID = uuid.New().String()

	p, err := s.repo.Create(ctx, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("create program: %w", err)
	}
	return p, nil
}

// Update validates and applies changes to the program with the given ID.
func (s *ProgramService) Update(ctx context.Context, id string, input models.ProgramUpdateInput) (*models.Program, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.Update")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", id))

	if input.Status != nil && !input.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status %q", domain.ErrInvalidInput, *input.Status)
	}
	if input.ProgramTermStatus != nil && !input.ProgramTermStatus.IsValid() {
		return nil, fmt.Errorf("%w: invalid program term status %q", domain.ErrInvalidInput, *input.ProgramTermStatus)
	}

	if input.Status != nil {
		current, err := s.repo.GetByID(ctx, id)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("get program for update: %w", err)
		}

		next := *input.Status
		allowed := programTransitions[current.Status]
		ok := false
		for _, s := range allowed {
			if s == next {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("%w: cannot transition program from %q to %q", domain.ErrInvalidStateTransition, current.Status, next)
		}

		// Submission guard (FR-004): all required fields must be present and at least one open term.
		if next == models.ProgramStatusSubmitted {
			if current.LFID == nil || strings.TrimSpace(*current.LFID) == "" {
				return nil, fmt.Errorf("%w: a linked LF project is required before submission", domain.ErrStateLocked)
			}
			if current.Description == nil || strings.TrimSpace(*current.Description) == "" {
				return nil, fmt.Errorf("%w: description is required before submission", domain.ErrStateLocked)
			}
			if current.RepoLink == nil || strings.TrimSpace(*current.RepoLink) == "" {
				return nil, fmt.Errorf("%w: repository URL is required before submission", domain.ErrStateLocked)
			}
			if current.LogoURL == nil || strings.TrimSpace(*current.LogoURL) == "" {
				return nil, fmt.Errorf("%w: logo is required before submission", domain.ErrStateLocked)
			}
			skills, err := s.repo.ListSkills(ctx, id)
			if err != nil {
				span.RecordError(err)
				return nil, fmt.Errorf("check program skills: %w", err)
			}
			if len(skills) == 0 {
				return nil, fmt.Errorf("%w: at least one skill tag is required before submission", domain.ErrStateLocked)
			}
			count, err := s.termRepo.CountOpenTermsByProgram(ctx, id)
			if err != nil {
				span.RecordError(err)
				return nil, fmt.Errorf("check open terms: %w", err)
			}
			if count == 0 {
				return nil, fmt.Errorf("%w: program must have at least one open term to be submitted", domain.ErrStateLocked)
			}
		}

		// Hide guard: must have no active applications.
		if next == models.ProgramStatusHidden {
			count, err := s.appRepo.CountBlockingAppsForProgram(ctx, id)
			if err != nil {
				span.RecordError(err)
				return nil, fmt.Errorf("check blocking applications: %w", err)
			}
			if count > 0 {
				return nil, fmt.Errorf("%w: program has %d active application(s) and cannot be hidden", domain.ErrStateLocked, count)
			}
		}
	}

	p, err := s.repo.Update(ctx, id, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("update program: %w", err)
	}
	return p, nil
}

// Delete removes the program with the given ID.
func (s *ProgramService) Delete(ctx context.Context, id string) error {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("program.id", id))

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete program: %w", err)
	}
	return nil
}

// ListSkills returns all skills for a program.
func (s *ProgramService) ListSkills(ctx context.Context, programID string) ([]*models.ProgramSkill, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.ListSkills")
	defer span.End()

	skills, err := s.repo.ListSkills(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("list skills: %w", err)
	}
	return skills, nil
}

// AddSkill adds a skill to a program.
func (s *ProgramService) AddSkill(ctx context.Context, programID string, input models.ProgramSkillCreateInput) (*models.ProgramSkill, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.AddSkill")
	defer span.End()

	if strings.TrimSpace(input.Skill) == "" {
		return nil, fmt.Errorf("%w: skill is required", domain.ErrInvalidInput)
	}

	skill, err := s.repo.AddSkill(ctx, programID, input)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("add skill: %w", err)
	}
	return skill, nil
}

// DeleteSkill removes a skill from a program.
func (s *ProgramService) DeleteSkill(ctx context.Context, skillID string) error {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.DeleteSkill")
	defer span.End()

	if err := s.repo.DeleteSkill(ctx, skillID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("delete skill: %w", err)
	}
	return nil
}

// GetFundingStats returns funding stats for a program.
func (s *ProgramService) GetFundingStats(ctx context.Context, programID string) (*models.ProgramFundingStats, error) {
	ctx, span := programSvcTracer.Start(ctx, "ProgramService.GetFundingStats")
	defer span.End()

	stats, err := s.repo.GetFundingStats(ctx, programID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("get funding stats: %w", err)
	}
	return stats, nil
}
