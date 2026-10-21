// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// syncPrerequisiteTasks brings the prerequisite tasks of the program's pending mentee applications
// in line with its prerequisites, inside the caller's transaction. A task is matched to the
// prerequisite of the same name, ignoring case and surrounding space. A task the mentee has not
// submitted takes its prerequisite's content, or is deleted when it has none; a submitted or
// complete task is kept as it is. A prerequisite no task matches is added as an incomplete task.
// An application left with every prerequisite task done is flagged tasks_submitted.
func syncPrerequisiteTasks(ctx context.Context, tx pgx.Tx, programID string, prerequisites []models.PrerequisiteTemplate) error {
	// Locking the applications first orders this write against task writes, which share-lock them.
	rows, err := tx.Query(ctx, `
		SELECT `+applicationCols+` FROM applications
		WHERE program_term_id IN (SELECT id FROM program_terms WHERE program_id = $1)
		  AND role = $2 AND status = $3
		FOR UPDATE`, programID, models.ApplicationRoleMentee, models.ApplicationStatusPending)
	if err != nil {
		return fmt.Errorf("lock pending applications for prerequisite sync: %w", err)
	}
	applications, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*models.Application, error) {
		return scanApplication(row)
	})
	if err != nil {
		return fmt.Errorf("load pending applications for prerequisite sync: %w", err)
	}
	if len(applications) == 0 {
		return nil
	}
	ids := make([]string, len(applications))
	for i, a := range applications {
		ids[i] = a.ID
	}
	rows, err = tx.Query(ctx, `
		SELECT `+taskCols+` FROM tasks
		WHERE application_id = ANY($1) AND category = $2
		ORDER BY created_on, id
		FOR UPDATE`, ids, models.TaskCategoryPrerequisite)
	if err != nil {
		return fmt.Errorf("lock prerequisite tasks for sync: %w", err)
	}
	tasks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*models.Task, error) {
		return scanTask(row)
	})
	if err != nil {
		return fmt.Errorf("load prerequisite tasks for sync: %w", err)
	}
	tasksByApplication := make(map[string][]*models.Task, len(applications))
	for _, t := range tasks {
		tasksByApplication[*t.ApplicationID] = append(tasksByApplication[*t.ApplicationID], t)
	}
	byKey := make(map[string]models.PrerequisiteTemplate, len(prerequisites))
	for _, p := range prerequisites {
		byKey[prerequisiteKey(&p.Name)] = p
	}

	for _, a := range applications {
		matched := make(map[string]bool, len(prerequisites))
		removed := false
		for _, t := range tasksByApplication[a.ID] {
			key := prerequisiteKey(t.Name)
			p, ok := byKey[key]
			if ok {
				matched[key] = true
			}
			if t.Status == models.TaskStatusSubmitted || t.Status == models.TaskStatusComplete {
				continue
			}
			if !ok {
				if err := deleteSyncedTask(ctx, tx, t); err != nil {
					return err
				}
				removed = true
				continue
			}
			if err := updateSyncedTask(ctx, tx, t, p); err != nil {
				return err
			}
		}

		var added []models.TaskCreateInput
		for _, p := range prerequisites {
			if matched[prerequisiteKey(&p.Name)] {
				continue
			}
			category := models.TaskCategoryPrerequisite
			termID, name, createdBy := a.ProgramTermID, p.Name, a.UserID
			added = append(added, models.TaskCreateInput{
				ID:            uuid.New().String(),
				ProgramTermID: &termID,
				AssigneeID:    a.UserID,
				Name:          &name,
				Description:   p.Description,
				SubmitFile:    p.SubmitFile,
				DueDate:       p.DueDate,
				Custom:        p.Custom,
				Category:      &category,
				Status:        models.TaskStatusIncomplete,
				CreatedBy:     &createdBy,
			})
		}
		if err := insertApplicationTasks(ctx, tx, a, added); err != nil {
			return err
		}
		// Only a removal can complete the set; the flag records the first completion and is never
		// cleared, so an added task leaves it as it is.
		if removed {
			if _, err := markTasksSubmitted(ctx, tx, a.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// prerequisiteKey is the name a task is matched to its prerequisite by.
func prerequisiteKey(name *string) string {
	if name == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*name))
}

// updateSyncedTask writes a prerequisite's content to its task, when any of it differs.
func updateSyncedTask(ctx context.Context, tx pgx.Tx, t *models.Task, p models.PrerequisiteTemplate) error {
	updated, err := scanTask(tx.QueryRow(ctx, `
		UPDATE tasks SET name = $2, description = $3, submit_file = $4, due_date = $5, custom = $6
		WHERE id = $1
		  AND (name, description, submit_file, due_date, custom) IS DISTINCT FROM ($2, $3, $4, $5, $6)
		RETURNING `+taskCols,
		t.ID, p.Name, p.Description, p.SubmitFile, p.DueDate, p.Custom))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sync prerequisite task %s: %w", t.ID, err)
	}
	return enqueueTaskIndex(ctx, tx, updated, "updated")
}

// deleteSyncedTask deletes a task whose prerequisite was removed, as TaskRepository.Delete does.
func deleteSyncedTask(ctx context.Context, tx pgx.Tx, t *models.Task) error {
	if _, err := tx.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, t.ID); err != nil {
		return fmt.Errorf("delete removed prerequisite task %s: %w", t.ID, err)
	}
	if err := queueObjectDeletions(ctx, tx, domain.ObjectBucketAttachments, t.File); err != nil {
		return err
	}
	if err := enqueueTaskMarker(ctx, tx, t, "delete_access"); err != nil {
		return err
	}
	return enqueueIndexDelete(ctx, tx, "mentorship_task", t.ID)
}
