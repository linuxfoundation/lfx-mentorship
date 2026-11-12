// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// term-status closes program terms that have ended once no pending or accepted mentee application
// is left, as legacy's update-program-terms-active job did. It runs four times a day as a CronJob.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("term-status failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()

	pool, err := db.NewPool(ctx, db.PoolConfig{
		MaxConns:        2,
		MinConns:        1,
		ConnMaxLifetime: 5 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()

	logger.Info("term-status starting")
	closed, err := service.NewTermStatusService(db.NewProgramTermRepository(pool), logger).CloseEndedTerms(ctx, start)
	if err != nil {
		return fmt.Errorf("closed %d term(s) before failing: %w", closed, err)
	}
	logger.Info("term-status complete",
		"duration", time.Since(start).String(),
		"closed", closed,
	)
	return nil
}
