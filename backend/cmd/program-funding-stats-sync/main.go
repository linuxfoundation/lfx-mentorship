// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// program-funding-stats-sync pulls mentorship credit transactions from the
// Ledger HTTP API and upserts per-program totals into program_funding_stats.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/clients"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("program-funding-stats-sync failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx := context.Background()
	start := time.Now()

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	pool, err := db.NewPool(ctx, db.PoolConfig{
		MaxConns:        cfg.DBMaxConns,
		MinConns:        cfg.DBMinConns,
		ConnMaxLifetime: cfg.DBConnMaxLifetime,
	})
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()

	ledgerClient := clients.NewLedgerClient(clients.LedgerConfig{
		BaseURL: cfg.LedgerBaseURL,
		APIKey:  cfg.LedgerAPIKey,
		Timeout: cfg.LedgerTimeout,
	})

	repo := db.NewProgramRepository(pool)
	syncer := newSyncer(repo, ledgerClient, logger, cfg.LedgerPerPage)
	syncer.dryRun = cfg.DryRun

	logger.Info("program-funding-stats-sync starting")
	result, err := syncer.Run(ctx)
	if err != nil {
		return fmt.Errorf("sync run: %w", err)
	}

	logger.Info("program-funding-stats-sync complete",
		"duration", time.Since(start).String(),
		"dry_run", cfg.DryRun,
		"programs", result.programs,
		"matched_programs", result.matchedPrograms,
		"skipped_programs", result.skippedPrograms,
		"processed_transactions", result.processedTxns,
		"unmapped_transactions", result.unmappedTxns,
		"pages_fetched", result.pagesFetched,
		"planned_upserts", result.plannedUpserts,
		"upserted", result.upserted,
	)
	return nil
}

type config struct {
	DBMaxConns        int
	DBMinConns        int
	DBConnMaxLifetime time.Duration
	LedgerBaseURL     string
	LedgerAPIKey      string
	LedgerTimeout     time.Duration
	LedgerPerPage     int
	DryRun            bool
}

func loadConfig() (*config, error) {
	ledgerBaseURL := requireEnv("LEDGER_BASE_URL")
	ledgerAPIKey := requireEnv("LEDGER_API_KEY")

	ledgerTimeout := 30 * time.Second
	if v := os.Getenv("LEDGER_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("LEDGER_TIMEOUT: invalid duration %q: %w", v, err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("LEDGER_TIMEOUT: must be greater than 0 (got %q)", v)
		}
		ledgerTimeout = d
	}

	perPage := 100
	if v := os.Getenv("LEDGER_PER_PAGE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("LEDGER_PER_PAGE: invalid integer %q: %w", v, err)
		}
		if n > 0 {
			perPage = n
		}
	}
	if perPage > 100 {
		perPage = 100
	}

	return &config{
		DBMaxConns:        5,
		DBMinConns:        1,
		DBConnMaxLifetime: 5 * time.Minute,
		LedgerBaseURL:     ledgerBaseURL,
		LedgerAPIKey:      ledgerAPIKey,
		LedgerTimeout:     ledgerTimeout,
		LedgerPerPage:     perPage,
		DryRun:            os.Getenv("DRY_RUN") == "true",
	}, nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "fatal: required environment variable %q is not set\n", key)
		os.Exit(1)
	}
	return v
}
