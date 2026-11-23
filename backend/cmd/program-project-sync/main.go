// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// program-project-sync refreshes each program's copy of its LF project's slug, name
// and logo from lfx-v2-project-service, and queues the changed programs for
// re-indexing, as legacy sync-lf-project-names did (issue #374). With DRY_RUN=true
// it reports what it would change without writing. It needs FGA_NATS_URL to reach
// Project Service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/projects"
	"github.com/nats-io/nats.go"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("program-project-sync failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()

	natsURL := os.Getenv("FGA_NATS_URL")
	if natsURL == "" {
		return errors.New("FGA_NATS_URL is required to reach lfx-v2-project-service")
	}
	dryRun := os.Getenv("DRY_RUN") == "true"

	pool, err := db.NewPool(ctx, db.PoolConfig{MaxConns: 2, MinConns: 1, ConnMaxLifetime: 5 * time.Minute})
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()

	conn, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("NATS connection: %w", err)
	}
	defer conn.Close()

	s := &syncer{
		store:    db.NewProgramRepository(pool),
		projects: projects.NewClient(conn),
		logger:   logger,
		dryRun:   dryRun,
	}
	logger.Info("program-project-sync starting", "dry_run", dryRun)
	result, err := s.Run(ctx)
	logger.Info("program-project-sync finished",
		"duration", time.Since(start).String(),
		"dry_run", dryRun,
		"projects", result.Projects,
		"programs", result.Programs,
		"unchanged_programs", result.Unchanged,
		"stale_programs", result.Stale,
		"updated_programs", result.Updated,
		"missing_projects", result.Missing,
		"failed_projects", result.Failed,
	)
	if err != nil {
		return err
	}
	if result.Failed > 0 {
		return fmt.Errorf("%d of %d projects could not be refreshed", result.Failed, result.Projects)
	}
	return nil
}
