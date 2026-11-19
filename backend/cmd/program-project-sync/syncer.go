// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
)

// programProjectStore is the part of the program repository the command uses.
type programProjectStore interface {
	ListProgramProjectMetadata(ctx context.Context) ([]db.ProgramProjectMetadata, error)
	SyncProjectMetadata(ctx context.Context, projectUID string, project models.ProjectMetadata) ([]string, error)
}

// projectSource reads an LF project from Project Service; domain.ProjectLookup satisfies it.
type projectSource interface {
	GetProject(ctx context.Context, uid string) (*models.ProjectMetadata, error)
}

// syncResult counts what one run did, or would do in a dry run. Projects counts
// distinct LF projects; Unchanged, Stale and Updated count programs.
type syncResult struct {
	Projects  int
	Programs  int
	Unchanged int
	Stale     int
	Updated   int
	Missing   int
	Failed    int
}

type syncer struct {
	store    programProjectStore
	projects projectSource
	logger   *slog.Logger
	dryRun   bool
}

// Run refreshes each LF project's slug, name and logo on its programs from Project Service.
// A project Project Service does not know is left as stored. A project that cannot be read
// or written is counted as failed and the run goes on; the caller reports a run with
// failures as failed. It stops with an error when the programs cannot be listed or the
// context ends.
func (s *syncer) Run(ctx context.Context) (syncResult, error) {
	programs, err := s.store.ListProgramProjectMetadata(ctx)
	if err != nil {
		return syncResult{}, fmt.Errorf("list program project metadata: %w", err)
	}
	result := syncResult{Programs: len(programs)}

	for _, group := range groupByProject(programs) {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("stopped after %d projects: %w", result.Projects, err)
		}
		result.Projects++
		uid := group[0].ProjectUID
		log := s.logger.With("project_uid", uid)

		project, err := s.projects.GetProject(ctx, uid)
		if errors.Is(err, domain.ErrProjectNotFound) {
			result.Missing++
			log.WarnContext(ctx, "project not found in Project Service; leaving its programs as stored", "programs", len(group))
			continue
		}
		if err != nil {
			result.Failed++
			log.ErrorContext(ctx, "read project from Project Service", "error", err)
			continue
		}

		stale := staleProgramIDs(group, project)
		result.Unchanged += len(group) - len(stale)
		if len(stale) == 0 {
			continue
		}
		result.Stale += len(stale)
		log = log.With("slug", project.Slug, "name", project.Name, "programs", len(stale))
		if s.dryRun {
			log.InfoContext(ctx, "dry run: would refresh project metadata", "program_ids", stale)
			continue
		}

		updated, err := s.store.SyncProjectMetadata(ctx, uid, *project)
		if err != nil {
			result.Failed++
			log.ErrorContext(ctx, "refresh project metadata", "error", err)
			continue
		}
		result.Updated += len(updated)
		log.InfoContext(ctx, "refreshed project metadata", "program_ids", updated)
	}
	return result, nil
}

// groupByProject splits programs, ordered by project UID, into one group per project.
func groupByProject(programs []db.ProgramProjectMetadata) [][]db.ProgramProjectMetadata {
	var groups [][]db.ProgramProjectMetadata
	for i, p := range programs {
		if i == 0 || p.ProjectUID != programs[i-1].ProjectUID {
			groups = append(groups, nil)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], p)
	}
	return groups
}

// staleProgramIDs returns the programs whose stored slug, name or logo differs from project,
// as SyncProjectMetadata compares them: an unset value differs from an empty one.
func staleProgramIDs(programs []db.ProgramProjectMetadata, project *models.ProjectMetadata) []string {
	var ids []string
	for _, p := range programs {
		if !equalValue(p.Slug, &project.Slug) || !equalValue(p.Name, &project.Name) || !equalValue(p.LogoURL, project.LogoURL) {
			ids = append(ids, p.ProgramID)
		}
	}
	return ids
}

func equalValue(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
