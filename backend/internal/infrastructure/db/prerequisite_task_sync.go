// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// applyPrerequisiteChanges applies an edit to the program's prerequisites to the prerequisite tasks
// of its pending mentee applications, inside the caller's transaction, and returns the
// applications it flagged tasks_submitted. Tasks are matched to prerequisites by PrerequisiteKey;
// only tasks cloned from a template are, which have no owner or are owned by their assignee, so a
// task a reviewer created for one applicant is never changed.
// Submitted and complete tasks are kept as they are. An unsubmitted task takes its edited
// prerequisite's content, and is deleted with its removed prerequisite unless the mentee has
// uploaded a file for it. An added prerequisite becomes an incomplete task on every application
// without one of that name.
func applyPrerequisiteChanges(ctx context.Context, tx pgx.Tx, programID string, changes models.PrerequisiteChanges) ([]string, error) {
	if changes.IsEmpty() {
		return nil, nil
	}
	// Lock the terms before the applications, as closing a term does, since inserting a task
	// key-share-locks its term; the other order lets the two deadlock.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM program_terms WHERE program_id = $1 FOR KEY SHARE`, programID); err != nil {
		return nil, fmt.Errorf("lock program terms for prerequisite changes: %w", err)
	}
	// Locking the applications first orders this write against task writes, which share-lock them.
	rows, err := tx.Query(ctx, `
		SELECT `+applicationCols+` FROM applications
		WHERE program_term_id IN (SELECT id FROM program_terms WHERE program_id = $1)
		  AND role = $2 AND status = $3
		FOR UPDATE`, programID, models.ApplicationRoleMentee, models.ApplicationStatusPending)
	if err != nil {
		return nil, fmt.Errorf("lock pending applications for prerequisite changes: %w", err)
	}
	applications, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*models.Application, error) {
		return scanApplication(row)
	})
	if err != nil {
		return nil, fmt.Errorf("load pending applications for prerequisite changes: %w", err)
	}
	if len(applications) == 0 {
		return nil, nil
	}
	ids := make([]string, len(applications))
	for i, a := range applications {
		ids[i] = a.ID
	}
	rows, err = tx.Query(ctx, `
		SELECT `+taskCols+` FROM tasks
		WHERE application_id = ANY($1) AND category = $2
		  AND (owner_id IS NULL OR owner_id = assignee_id)
		FOR UPDATE`, ids, models.TaskCategoryPrerequisite)
	if err != nil {
		return nil, fmt.Errorf("lock prerequisite tasks for changes: %w", err)
	}
	tasks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*models.Task, error) {
		return scanTask(row)
	})
	if err != nil {
		return nil, fmt.Errorf("load prerequisite tasks for changes: %w", err)
	}
	tasksByApplication := make(map[string][]*models.Task, len(applications))
	for _, t := range tasks {
		tasksByApplication[*t.ApplicationID] = append(tasksByApplication[*t.ApplicationID], t)
	}
	removed := make(map[string]bool, len(changes.Removed))
	for _, key := range changes.Removed {
		removed[key] = true
	}
	edited := make(map[string]models.PrerequisiteTemplate, len(changes.Edited))
	for _, p := range changes.Edited {
		edited[models.PrerequisiteKey(p.Name)] = p
	}

	var flagged []string
	for _, a := range applications {
		present := make(map[string]bool, len(tasksByApplication[a.ID]))
		deleted := false
		for _, t := range tasksByApplication[a.ID] {
			key := ""
			if t.Name != nil {
				key = models.PrerequisiteKey(*t.Name)
			}
			present[key] = true
			if t.Status == models.TaskStatusSubmitted || t.Status == models.TaskStatusComplete {
				continue
			}
			if p, ok := edited[key]; ok {
				if err := updatePrerequisiteTask(ctx, tx, t, p); err != nil {
					return nil, err
				}
			} else if removed[key] && (t.File == nil || *t.File == "") {
				if err := deleteTaskInTx(ctx, tx, t); err != nil {
					return nil, err
				}
				deleted = true
			}
		}

		var added []models.TaskCreateInput
		for _, p := range changes.Added {
			if !present[models.PrerequisiteKey(p.Name)] {
				added = append(added, p.NewTask(uuid.New().String(), a.ProgramTermID, a.UserID))
			}
		}
		if err := insertApplicationTasks(ctx, tx, a, added); err != nil {
			return nil, err
		}
		// Only a deletion can complete the set; the flag records the first completion and is never
		// cleared, so an added task leaves it as it is. A deletion that leaves no prerequisite has
		// removed the last one outstanding, so that completes the set too.
		if deleted {
			flipped, err := markTasksSubmitted(ctx, tx, a.ID, true)
			if err != nil {
				return nil, err
			}
			if flipped {
				flagged = append(flagged, a.ID)
			}
		}
	}
	return flagged, nil
}

// updatePrerequisiteTask writes an edited prerequisite's content to its task, when any of it differs.
func updatePrerequisiteTask(ctx context.Context, tx pgx.Tx, t *models.Task, p models.PrerequisiteTemplate) error {
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
		return fmt.Errorf("apply edited prerequisite to task %s: %w", t.ID, err)
	}
	return enqueueTaskIndex(ctx, tx, updated, "updated")
}
