// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type ApplicationIndexDocument struct {
	ID                string                    `json:"id"`
	ProgramTermID     string                    `json:"program_term_id"`
	UserID            string                    `json:"user_id"`
	Role              models.ApplicationRole    `json:"role"`
	Status            models.ApplicationStatus  `json:"status"`
	ProgramTermStatus *models.ProgramTermStatus `json:"program_term_status,omitempty"`
	StartDateTime     *time.Time                `json:"start_date_time,omitempty"`
	EndDateTime       *time.Time                `json:"end_date_time,omitempty"`
	AttendanceType    *models.AttendanceType    `json:"attendance_type,omitempty"`
	TasksSubmitted    bool                      `json:"tasks_submitted"`
	CreatedOn         time.Time                 `json:"created_on"`
	UpdatedOn         time.Time                 `json:"updated_on"`
}

func NewApplicationIndexDocument(application *models.Application) ApplicationIndexDocument {
	return ApplicationIndexDocument{
		ID:                application.ID,
		ProgramTermID:     application.ProgramTermID,
		UserID:            application.UserID,
		Role:              application.Role,
		Status:            application.Status,
		ProgramTermStatus: application.ProgramTermStatus,
		StartDateTime:     application.StartDateTime,
		EndDateTime:       application.EndDateTime,
		AttendanceType:    application.AttendanceType,
		TasksSubmitted:    application.TasksSubmitted,
		CreatedOn:         application.CreatedOn,
		UpdatedOn:         application.UpdatedOn,
	}
}

func NewApplicationIndexConfig(applicationID, programID, userID, role, status string) map[string]any {
	return map[string]any{
		"object_id":              applicationID,
		"object_ref":             "mentorship_application:" + applicationID,
		"object_type":            "mentorship_application",
		"access_check_object":    "mentorship_application:" + applicationID,
		"access_check_relation":  "auditor",
		"history_check_object":   "mentorship_program:" + programID,
		"history_check_relation": "auditor",
		"sort_name":              userID,
		"name_and_aliases":       []string{userID, role},
		"public":                 false,
		"tags":                   []string{"role:" + role, "status:" + status},
	}
}

type TaskIndexDocument struct {
	ID                string                    `json:"id"`
	ApplicationID     *string                   `json:"application_id,omitempty"`
	AssigneeID        string                    `json:"assignee_id"`
	Name              *string                   `json:"name,omitempty"`
	Description       *string                   `json:"description,omitempty"`
	Category          *models.TaskCategory      `json:"category,omitempty"`
	Prerequisite      bool                      `json:"prerequisite"`
	Status            models.TaskStatus         `json:"status"`
	ApplicationStatus *models.ApplicationStatus `json:"application_status,omitempty"`
	ProgramTermStatus *models.ProgramTermStatus `json:"program_term_status,omitempty"`
	Custom            bool                      `json:"custom"`
	SubmitFile        *string                   `json:"submit_file,omitempty"`
	File              *string                   `json:"file,omitempty"`
	DueDate           *string                   `json:"due_date,omitempty"`
	CreatedOn         time.Time                 `json:"created_on"`
	UpdatedOn         time.Time                 `json:"updated_on"`
}

func NewTaskIndexDocument(task *models.Task) TaskIndexDocument {
	return TaskIndexDocument{
		ID:                task.ID,
		ApplicationID:     task.ApplicationID,
		AssigneeID:        task.AssigneeID,
		Name:              task.Name,
		Description:       task.Description,
		Category:          task.Category,
		Prerequisite:      task.Category != nil && *task.Category == models.TaskCategoryPrerequisite,
		Status:            task.Status,
		ApplicationStatus: task.ApplicationStatus,
		ProgramTermStatus: task.ProgramTermStatus,
		Custom:            task.Custom,
		SubmitFile:        task.SubmitFile,
		File:              task.File,
		DueDate:           task.DueDate,
		CreatedOn:         task.CreatedOn,
		UpdatedOn:         task.UpdatedOn,
	}
}

func NewTaskIndexConfig(taskID, applicationID, assigneeID, name, category, status string) map[string]any {
	return map[string]any{
		"object_id":              taskID,
		"object_ref":             "mentorship_task:" + taskID,
		"object_type":            "mentorship_task",
		"access_check_object":    "mentorship_task:" + taskID,
		"access_check_relation":  "auditor",
		"history_check_object":   "mentorship_application:" + applicationID,
		"history_check_relation": "auditor",
		"sort_name":              name,
		"name_and_aliases":       []string{name, category},
		"public":                 false,
		"tags":                   []string{"status:" + status, "category:" + category, "assignee_id:" + assigneeID},
	}
}

func enqueueApplicationIndex(ctx context.Context, tx pgx.Tx, application *models.Application, action string) error {
	var programID string
	if err := tx.QueryRow(ctx, `SELECT program_id FROM program_terms WHERE id = $1`, application.ProgramTermID).Scan(&programID); err != nil {
		return fmt.Errorf("resolve application index parent: %w", err)
	}
	return enqueueIndexDocument(ctx, tx, "mentorship_application", application.ID, action,
		NewApplicationIndexDocument(application),
		NewApplicationIndexConfig(application.ID, programID, application.UserID, string(application.Role), string(application.Status)))
}

func enqueueTaskIndex(ctx context.Context, tx pgx.Tx, task *models.Task, action string) error {
	if task.ApplicationID == nil || *task.ApplicationID == "" {
		return fmt.Errorf("task %s has no application parent for indexing", task.ID)
	}
	name, category := "", ""
	if task.Name != nil {
		name = *task.Name
	}
	if task.Category != nil {
		category = string(*task.Category)
	}
	return enqueueIndexDocument(ctx, tx, "mentorship_task", task.ID, action, NewTaskIndexDocument(task),
		NewTaskIndexConfig(task.ID, *task.ApplicationID, task.AssigneeID, name, category, string(task.Status)))
}

func enqueueIndexDocument(ctx context.Context, tx pgx.Tx, objectType, objectID, action string, document any, config map[string]any) error {
	headers, err := json.Marshal(domain.SanitizedIndexHeaders(domain.IndexHeadersFromContext(ctx)))
	if err != nil {
		return fmt.Errorf("marshal index headers: %w", err)
	}
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("marshal %s index document: %w", objectType, err)
	}
	indexingConfig, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal %s index config: %w", objectType, err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO index_outbox (object_type, object_uid, action, headers, data, indexing_config)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (object_type, object_uid) DO UPDATE SET
			action = EXCLUDED.action,
			headers = EXCLUDED.headers,
			data = EXCLUDED.data,
			indexing_config = EXCLUDED.indexing_config,
			generation = index_outbox.generation + 1,
			state = CASE WHEN index_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
			claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
			claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
			attempts = 0,
			next_attempt_at = NOW(),
			sent_on = NULL`, objectType, objectID, action, headers, data, indexingConfig)
	if err != nil {
		return fmt.Errorf("enqueue %s index record: %w", objectType, err)
	}
	return nil
}

func enqueueIndexDelete(ctx context.Context, tx pgx.Tx, objectType, objectID string) error {
	headers, err := json.Marshal(domain.SanitizedIndexHeaders(domain.IndexHeadersFromContext(ctx)))
	if err != nil {
		return fmt.Errorf("marshal index headers: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO index_outbox (object_type, object_uid, action, headers)
		VALUES ($1, $2, 'deleted', $3)
		ON CONFLICT (object_type, object_uid) DO UPDATE SET
			action = 'deleted',
			headers = EXCLUDED.headers,
			data = NULL,
			indexing_config = NULL,
			generation = index_outbox.generation + 1,
			state = CASE WHEN index_outbox.state = 'in_flight' THEN 'in_flight' ELSE 'pending' END,
			claimed_generation = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_generation ELSE NULL END,
			claimed_at = CASE WHEN index_outbox.state = 'in_flight' THEN index_outbox.claimed_at ELSE NULL END,
			attempts = 0,
			next_attempt_at = NOW(),
			sent_on = NULL`, objectType, objectID, headers)
	if err != nil {
		return fmt.Errorf("enqueue %s index deletion: %w", objectType, err)
	}
	return nil
}
