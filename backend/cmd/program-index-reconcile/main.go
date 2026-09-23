// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Command program-index-reconcile queues index snapshots for all current programs.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
)

type programSnapshot struct {
	ID         string    `json:"id"`
	ProjectUID *string   `json:"project_uid,omitempty"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Status     string    `json:"status"`
	LogoURL    *string   `json:"logo_url,omitempty"`
	CreatedOn  time.Time `json:"created_on"`
	UpdatedOn  time.Time `json:"updated_on"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(context.Background(), os.Getenv("INDEXER_AUTHORIZATION")); err != nil {
		logger.Error("program index reconciliation failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, authorization string) error {
	if authorization == "" {
		return fmt.Errorf("INDEXER_AUTHORIZATION is required")
	}
	pool, err := db.NewPool(ctx, db.PoolConfig{MaxConns: 10, MinConns: 2})
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()
	rows, err := pool.Query(ctx, `SELECT id, project_uid, name, slug, status, logo_url, created_on, updated_on FROM programs`)
	if err != nil {
		return fmt.Errorf("list programs: %w", err)
	}
	defer rows.Close()
	outbox := db.NewIndexOutboxRepository(pool)
	for rows.Next() {
		var program programSnapshot
		if err := rows.Scan(&program.ID, &program.ProjectUID, &program.Name, &program.Slug, &program.Status, &program.LogoURL, &program.CreatedOn, &program.UpdatedOn); err != nil {
			return fmt.Errorf("scan program: %w", err)
		}
		data, err := json.Marshal(program)
		if err != nil {
			return err
		}
		config := map[string]any{
			"object_id": program.ID, "access_check_object": "mentorship_program:" + program.ID,
			"access_check_relation": "writer", "history_check_object": "mentorship_program:" + program.ID,
			"history_check_relation": "auditor", "sort_name": program.Name,
			"name_and_aliases": []string{program.Name, program.Slug}, "public": program.Status == "published",
			"tags": []string{"status:" + program.Status},
		}
		if program.ProjectUID != nil {
			config["parent_refs"] = []string{"project:" + *program.ProjectUID}
			config["tags"] = []string{"status:" + program.Status, "project_uid:" + *program.ProjectUID}
		}
		configData, err := json.Marshal(config)
		if err != nil {
			return err
		}
		headers, _ := json.Marshal(map[string]string{"authorization": authorization})
		if err := outbox.Enqueue(ctx, domain.IndexOutboxRecord{ObjectType: "mentorship_program", ObjectUID: program.ID, Action: "updated", Headers: headers, Data: data, IndexingConfig: configData}); err != nil {
			return err
		}
	}
	return rows.Err()
}
