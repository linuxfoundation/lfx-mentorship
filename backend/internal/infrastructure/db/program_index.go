// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package db

import (
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

type ProgramIndexDocument struct {
	ID         string                    `json:"id"`
	ProjectUID *string                   `json:"project_uid,omitempty"`
	Name       string                    `json:"name"`
	Slug       string                    `json:"slug"`
	Status     string                    `json:"status"`
	LogoURL    *string                   `json:"logo_url,omitempty"`
	Stats      models.ProgramHeaderStats `json:"stats"`
	CreatedOn  time.Time                 `json:"created_on"`
	UpdatedOn  time.Time                 `json:"updated_on"`
}

func NewProgramIndexDocument(program *models.Program) ProgramIndexDocument {
	return ProgramIndexDocument{
		ID:         program.ID,
		ProjectUID: program.ProjectUID,
		Name:       program.Name,
		Slug:       program.Slug,
		Status:     string(program.Status),
		LogoURL:    program.LogoURL,
		CreatedOn:  program.CreatedOn,
		UpdatedOn:  program.UpdatedOn,
	}
}

func NewProgramIndexConfig(id string, projectUID *string, name, slug, status string) map[string]any {
	config := map[string]any{
		"object_id":              id,
		"object_ref":             "mentorship_program:" + id,
		"object_type":            "mentorship_program",
		"access_check_object":    "mentorship_program:" + id,
		"access_check_relation":  "viewer",
		"history_check_object":   "mentorship_program:" + id,
		"history_check_relation": "auditor",
		"sort_name":              name,
		"name_and_aliases":       []string{name, slug},
		"public":                 status == string(models.ProgramStatusPublished),
		"tags":                   []string{"status:" + status},
	}
	if projectUID != nil {
		config["parent_refs"] = []string{"project:" + *projectUID}
		config["tags"] = []string{"status:" + status, "project_uid:" + *projectUID}
	}
	return config
}
