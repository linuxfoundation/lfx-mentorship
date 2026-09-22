// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_DSN: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = "mentorship,public"
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE fga_outbox RESTART IDENTITY"); err != nil {
		t.Fatalf("reset fga_outbox: %v", err)
	}
	return pool
}

func TestFGAOutboxIntegration_ClaimSerializesSameObject(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-1", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	start := make(chan struct{})
	results := make(chan []domain.FGAOutboxMarker, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			markers, err := repo.Claim(ctx, 10)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			results <- markers
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	claimed := 0
	for markers := range results {
		claimed += len(markers)
	}
	if claimed != 1 {
		t.Fatalf("claimed %d markers; want exactly one", claimed)
	}
}

func TestFGAOutboxIntegration_NewGenerationCannotBeAcknowledgedByOldClaim(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-2", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	old := markers[0]

	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-2", "update_access"); err != nil {
		t.Fatalf("enqueue newer generation: %v", err)
	}
	acknowledged, err := repo.Acknowledge(ctx, old)
	if err != nil {
		t.Fatalf("acknowledge old generation: %v", err)
	}
	if acknowledged {
		t.Fatal("old generation was acknowledged after a newer enqueue")
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM fga_outbox WHERE id = $1`, old.ID).Scan(&state); err != nil {
		t.Fatalf("read newer generation state: %v", err)
	}
	if state != "pending" {
		t.Fatalf("newer generation state = %q; want pending", state)
	}

	fresh, err := repo.Claim(ctx, 1)
	if err != nil || len(fresh) != 1 {
		t.Fatalf("claim fresh generation: markers=%d err=%v", len(fresh), err)
	}
	if fresh[0].Generation <= old.Generation {
		t.Fatalf("generation = %d; want greater than %d", fresh[0].Generation, old.Generation)
	}
}

func TestFGAOutboxIntegration_StaleClaimCanBeReclaimed(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_task", "task-1", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fga_outbox SET claimed_at = NOW() - INTERVAL '6 minutes' WHERE id = $1`, markers[0].ID); err != nil {
		t.Fatalf("age claim: %v", err)
	}
	fresh, err := repo.Claim(ctx, 1)
	if err != nil || len(fresh) != 1 {
		t.Fatalf("reclaim stale marker: markers=%d err=%v", len(fresh), err)
	}
	if fresh[0].ID != markers[0].ID {
		t.Fatalf("reclaimed ID = %d; want %d", fresh[0].ID, markers[0].ID)
	}
}

func TestFGAOutboxIntegration_DeadLetterPreservesFailure(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_application", "application-1", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.DeadLetter(ctx, markers[0], "permanent builder failure"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}
	var state, lastError string
	if err := pool.QueryRow(ctx, `SELECT state, last_error FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state, &lastError); err != nil {
		t.Fatalf("read dead-letter marker: %v", err)
	}
	if state != "dead_letter" || lastError != "permanent builder failure" {
		t.Fatalf("state=%q last_error=%q; want dead_letter and preserved error", state, lastError)
	}
}

func TestFGAOutboxIntegration_ReplayDeadLetter(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-replay", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.DeadLetter(ctx, markers[0], "permanent failure"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}
	if err := repo.ReplayDeadLetter(ctx, markers[0].ID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	fresh, err := repo.Claim(ctx, 1)
	if err != nil || len(fresh) != 1 {
		t.Fatalf("claim replayed marker: markers=%d err=%v", len(fresh), err)
	}
	if fresh[0].ID != markers[0].ID || fresh[0].Attempts != 0 {
		t.Fatalf("replayed marker = %+v; want original ID and zero attempts", fresh[0])
	}
}

func TestFGAOutboxIntegration_AcknowledgeHandlesNullClaimedAt(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()

	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO fga_outbox (
			marker_kind, object_type, object_uid, desired_operation,
			state, generation, claimed_generation, claimed_at
		)
		VALUES ('object', 'mentorship_program', 'program-null-claim', 'update_access', 'in_flight', 1, 1, NULL)
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert null-claimed marker: %v", err)
	}

	acknowledged, err := repo.Acknowledge(ctx, domain.FGAOutboxMarker{ID: id, Generation: 1, ClaimedAt: nil})
	if err != nil {
		t.Fatalf("acknowledge with nil claimed_at: %v", err)
	}
	if !acknowledged {
		t.Fatal("expected acknowledge to delete matching marker with nil claimed_at")
	}

	var remaining int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM fga_outbox WHERE id = $1`, id).Scan(&remaining); err != nil {
		t.Fatalf("count marker after acknowledge: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining markers = %d; want 0", remaining)
	}
}

func TestFGAOutboxIntegration_ReconcileObjectPreservesDeadLetterState(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()

	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-dead-reconcile", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.DeadLetter(ctx, markers[0], "builder failure"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}

	if err := repo.ReconcileObject(ctx, "mentorship_program", "program-dead-reconcile", "update_access"); err != nil {
		t.Fatalf("reconcile object: %v", err)
	}

	var state, lastError string
	if err := pool.QueryRow(ctx, `SELECT state, last_error FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state, &lastError); err != nil {
		t.Fatalf("read reconciled marker: %v", err)
	}
	if state != "dead_letter" {
		t.Fatalf("state = %q; want dead_letter", state)
	}
	if lastError != "builder failure" {
		t.Fatalf("last_error = %q; want builder failure", lastError)
	}
}

func TestFGAOutboxIntegration_ReconcileMembershipPreservesDeadLetterState(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()

	if err := repo.EnqueueMembership(ctx, "mentorship_program", "program-dead-membership", "mentor", "alice"); err != nil {
		t.Fatalf("enqueue membership: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim: markers=%d err=%v", len(markers), err)
	}
	if err := repo.DeadLetter(ctx, markers[0], "membership builder failure"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}

	if err := repo.ReconcileMembership(ctx, "mentorship_program", "program-dead-membership", "mentor", "alice"); err != nil {
		t.Fatalf("reconcile membership: %v", err)
	}

	var state, lastError string
	if err := pool.QueryRow(ctx, `SELECT state, last_error FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state, &lastError); err != nil {
		t.Fatalf("read reconciled membership marker: %v", err)
	}
	if state != "dead_letter" {
		t.Fatalf("state = %q; want dead_letter", state)
	}
	if lastError != "membership builder failure" {
		t.Fatalf("last_error = %q; want membership builder failure", lastError)
	}
}
