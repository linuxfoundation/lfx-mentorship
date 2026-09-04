// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package db provides PostgreSQL connection helpers and repositories.
package db

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig holds connection pool settings sourced from environment variables.
// Connection details are read from the environment by ConnConfigFromEnv rather
// than passed as a DSN string, so credentials are never string-interpolated.
type PoolConfig struct {
	MaxConns        int
	MinConns        int
	ConnMaxLifetime time.Duration
}

// NewPool creates a pgxpool.Pool, pings the database, and returns it.
func NewPool(ctx context.Context, cfg PoolConfig) (*pgxpool.Pool, error) {
	connConfig, err := ConnConfigFromEnv()
	if err != nil {
		return nil, err
	}
	config, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("build pool config: %w", err)
	}
	config.ConnConfig = connConfig
	if cfg.MaxConns < 0 || cfg.MaxConns > math.MaxInt32 {
		return nil, fmt.Errorf("MaxConns %d is out of valid range [0, %d]", cfg.MaxConns, math.MaxInt32)
	}
	if cfg.MinConns < 0 || cfg.MinConns > math.MaxInt32 {
		return nil, fmt.Errorf("MinConns %d is out of valid range [0, %d]", cfg.MinConns, math.MaxInt32)
	}
	if cfg.MaxConns > 0 && cfg.MinConns > cfg.MaxConns {
		return nil, fmt.Errorf("invalid pool: DB_MIN_CONNS (%d) must be ≤ DB_MAX_CONNS (%d)", cfg.MinConns, cfg.MaxConns)
	}
	config.MaxConns = int32(cfg.MaxConns)
	config.MinConns = int32(cfg.MinConns)
	config.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
