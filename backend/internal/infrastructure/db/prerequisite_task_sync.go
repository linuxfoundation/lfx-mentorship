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
// of the pending mentee applications on its open terms, inside the caller's transaction, and
// returns the applications it flagged tasks_submitted. Closing a term declines its pending
// applications, so one still pending on a closed term is stale imported data and is left alone. Tasks are matched to prerequisites by PrerequisiteKey.
// Only tasks cloned from a template are changed (see clonedFromTemplate), so a task a reviewer or
// service created for one applicant never is, though its name still counts as present.
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
	var legacyCreator *string
	if err := tx.QueryRow(ctx, `SELECT lfid FROM programs WHERE id = $1`, programID).Scan(&legacyCreator); err != nil {
		return nil, fmt.Errorf("load program creator for prerequisite changes: %w", err)
	}
	// Locking the applications first orders this write against task writes, which share-lock them.
	// The term lock above holds off a concurrent close, which locks its term for update.
	rows, err := tx.Query(ctx, `
		SELECT `+applicationCols+` FROM applications
		WHERE program_term_id IN (SELECT id FROM program_terms WHERE program_id = $1 AND status = 'open')
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
		flipped, err := syncApplicationPrerequisiteTasks(ctx, tx, a, tasksByApplication[a.ID], legacyCreator,
			func(key string) (models.PrerequisiteTemplate, bool) { p, ok := edited[key]; return p, ok },
			func(key string) bool { return removed[key] },
			changes.Added)
		if err != nil {
			return nil, err
		}
		// The program update refreshes the program's search document once, after every application.
		if flipped {
			flagged = append(flagged, a.ID)
		}
	}
	return flagged, nil
}

// reconcilePrerequisiteTasks brings a re-opened mentee application's prerequisite tasks in line
// with the program's prerequisites, inside the caller's transaction, and reports whether it flagged
// the application tasks_submitted. Prerequisite edits reach only pending applications
// (applyPrerequisiteChanges), so one declined or withdrawn meanwhile missed them: a task whose
// prerequisite is gone counts as removed, one whose prerequisite remains takes its content, and a
// prerequisite with no task gets one, by applyPrerequisiteChanges's rules. Deleting the last
// outstanding task flags the application, but notifies no one: re-opening sends nothing.
func reconcilePrerequisiteTasks(ctx context.Context, tx pgx.Tx, application *models.Application, programID string, prerequisites []models.PrerequisiteTemplate) (bool, error) {
	var legacyCreator *string
	if err := tx.QueryRow(ctx, `SELECT lfid FROM programs WHERE id = $1`, programID).Scan(&legacyCreator); err != nil {
		return false, fmt.Errorf("load program creator for re-open: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT `+taskCols+` FROM tasks
		WHERE application_id = $1 AND category = $2
		FOR UPDATE`, application.ID, models.TaskCategoryPrerequisite)
	if err != nil {
		return false, fmt.Errorf("lock prerequisite tasks for re-open: %w", err)
	}
	tasks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*models.Task, error) {
		return scanTask(row)
	})
	if err != nil {
		return false, fmt.Errorf("load prerequisite tasks for re-open: %w", err)
	}
	current := make(map[string]models.PrerequisiteTemplate, len(prerequisites))
	for _, p := range prerequisites {
		current[models.PrerequisiteKey(p.Name)] = p
	}
	return syncApplicationPrerequisiteTasks(ctx, tx, application, tasks, legacyCreator,
		func(key string) (models.PrerequisiteTemplate, bool) { p, ok := current[key]; return p, ok },
		func(key string) bool { _, ok := current[key]; return !ok },
		prerequisites)
}

// syncApplicationPrerequisiteTasks applies prerequisite content to one application's locked
// prerequisite tasks and reports whether it flagged the application tasks_submitted. Tasks are
// matched to prerequisites by PrerequisiteKey. Only tasks cloned from a template are changed (see
// clonedFromTemplate), so a task a reviewer or service created for one applicant never is, though
// its name still counts as present. Submitted and complete tasks are kept as they are. An
// unsubmitted task takes the content edited returns for it, and is deleted when removed reports its
// prerequisite gone, unless the mentee has uploaded a file for it. Each of added with no task of its
// name becomes an incomplete task.
func syncApplicationPrerequisiteTasks(
	ctx context.Context, tx pgx.Tx, a *models.Application, tasks []*models.Task, legacyCreator *string,
	edited func(key string) (models.PrerequisiteTemplate, bool), removed func(key string) bool, added []models.PrerequisiteTemplate,
) (bool, error) {
	present := make(map[string]bool, len(tasks))
	deleted := false
	for _, t := range tasks {
		key := ""
		if t.Name != nil {
			key = models.PrerequisiteKey(*t.Name)
		}
		present[key] = true
		if !clonedFromTemplate(t, legacyCreator) || t.Status == models.TaskStatusSubmitted || t.Status == models.TaskStatusComplete {
			continue
		}
		if p, ok := edited(key); ok {
			if err := updatePrerequisiteTask(ctx, tx, t, p); err != nil {
				return false, err
			}
		} else if removed(key) && (t.File == nil || *t.File == "") {
			if err := deleteTaskInTx(ctx, tx, t); err != nil {
				return false, err
			}
			deleted = true
		}
	}

	var inserts []models.TaskCreateInput
	for _, p := range added {
		if !present[models.PrerequisiteKey(p.Name)] {
			inserts = append(inserts, p.NewTask(uuid.New().String(), a.ProgramTermID, a.UserID))
		}
	}
	if err := insertApplicationTasks(ctx, tx, a, inserts); err != nil {
		return false, err
	}
	// Only a deletion can complete the set; the flag records the first completion and is never
	// cleared, so an added task leaves it as it is. A deletion that leaves no prerequisite has
	// removed the last one outstanding, so that completes the set too.
	if !deleted {
		return false, nil
	}
	return markTasksSubmitted(ctx, tx, a.ID, true)
}

// clonedFromTemplate reports whether a prerequisite task was cloned from its program's templates.
// A clone made here has no owner and was created by its applicant (models.PrerequisiteTemplate.NewTask),
// which no task route can produce: a route records its caller as creator, and as owner unless the
// caller is a service. Legacy cloned templates as the program's creator, whose LFID it recorded as
// created_by and the program keeps as lfid, which cannot change once set; tasks created through
// routes record a user ID instead.
func clonedFromTemplate(t *models.Task, legacyCreator *string) bool {
	if t.CreatedBy == nil {
		return false
	}
	if t.OwnerID == nil && *t.CreatedBy == t.AssigneeID {
		return true
	}
	return legacyCreator != nil && *legacyCreator != "" && *t.CreatedBy == *legacyCreator
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
