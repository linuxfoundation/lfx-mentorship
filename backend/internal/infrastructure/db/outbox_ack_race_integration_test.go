// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

// whileEnqueueHoldsRowLock bumps the row's generation in an open transaction, as a
// concurrent enqueue does, runs ack, and once ack is blocked on that lock runs onBlocked
// (when set) and commits.
func whileEnqueueHoldsRowLock(t *testing.T, pool *pgxpool.Pool, table string, id any, ack, onBlocked func()) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin enqueue: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE `+table+` SET generation = generation + 1 WHERE id = $1`, id); err != nil {
		t.Fatalf("enqueue newer generation: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ack()
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatalf("read lock waits: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("acknowledgement never waited on the enqueue's row lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if onBlocked != nil {
		onBlocked()
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit enqueue: %v", err)
	}
	<-done
}

// eventuallyNoLockWaiters waits until the server has aborted every statement blocked on a row lock.
func eventuallyNoLockWaiters(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatalf("read lock waits: %v", err)
		}
		if waiting == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("cancelled statement is still waiting on the row lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestIndexOutboxIntegration_MarkSentRequeuesGenerationCommittedDuringAck(t *testing.T) {
	pool := integrationPool(t)
	repo := NewIndexOutboxRepository(pool)
	ctx := context.Background()
	record := domain.IndexOutboxRecord{ObjectType: "mentorship_program", ObjectUID: "00000000-0000-0000-0000-000000000010", Action: "updated", Data: []byte(`{}`)}
	if err := repo.Enqueue(ctx, record); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim records=%d err=%v", len(claimed), err)
	}
	var acknowledged bool
	whileEnqueueHoldsRowLock(t, pool, "index_outbox", claimed[0].ID, func() {
		acknowledged, err = repo.MarkSent(ctx, claimed[0])
	}, nil)
	if err != nil || !acknowledged {
		t.Fatalf("mark sent: acknowledged=%v err=%v", acknowledged, err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM index_outbox WHERE id = $1`, claimed[0].ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "pending" {
		t.Fatalf("state = %q; want the newer generation pending", state)
	}
}

func TestIndexOutboxIntegration_MarkRetryRequeuesGenerationCommittedDuringAck(t *testing.T) {
	pool := integrationPool(t)
	repo := NewIndexOutboxRepository(pool)
	repo.SetMaxAttempts(1)
	ctx := context.Background()
	record := domain.IndexOutboxRecord{ObjectType: "mentorship_program", ObjectUID: "00000000-0000-0000-0000-000000000010", Action: "updated", Data: []byte(`{}`)}
	if err := repo.Enqueue(ctx, record); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := repo.Claim(ctx, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim records=%d err=%v", len(claimed), err)
	}
	var acknowledged, deadLettered bool
	whileEnqueueHoldsRowLock(t, pool, "index_outbox", claimed[0].ID, func() {
		acknowledged, deadLettered, err = repo.MarkRetry(ctx, claimed[0])
	}, nil)
	if err != nil || !acknowledged || deadLettered {
		t.Fatalf("mark retry: acknowledged=%v dead_lettered=%v err=%v", acknowledged, deadLettered, err)
	}
	var state string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT state, attempts FROM index_outbox WHERE id = $1`, claimed[0].ID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	// The failure belonged to the old generation, so the new one is neither dead-lettered nor charged an attempt.
	if state != "pending" || attempts != 0 {
		t.Fatalf("state=%q attempts=%d; want pending with no attempts", state, attempts)
	}
}

func TestFGAOutboxIntegration_AcknowledgeRequeuesGenerationCommittedDuringAck(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-race", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim markers=%d err=%v", len(markers), err)
	}
	var acknowledged bool
	whileEnqueueHoldsRowLock(t, pool, "fga_outbox", markers[0].ID, func() {
		acknowledged, err = repo.Acknowledge(ctx, markers[0])
	}, nil)
	if err != nil || acknowledged {
		t.Fatalf("acknowledge: acknowledged=%v err=%v; want the old generation unacknowledged", acknowledged, err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "pending" {
		t.Fatalf("state = %q; want the newer generation pending", state)
	}
}

func TestFGAOutboxIntegration_AcknowledgeCancelledMidWaitLeavesNoPartialState(t *testing.T) {
	pool := integrationPool(t)
	repo := NewFGAOutboxRepository(pool)
	ctx := context.Background()
	if err := repo.EnqueueObject(ctx, "mentorship_program", "program-cancel", "update_access"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	markers, err := repo.Claim(ctx, 1)
	if err != nil || len(markers) != 1 {
		t.Fatalf("claim markers=%d err=%v", len(markers), err)
	}
	ackCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	returned := make(chan struct{})
	whileEnqueueHoldsRowLock(t, pool, "fga_outbox", markers[0].ID, func() {
		defer close(returned)
		_, err = repo.Acknowledge(ackCtx, markers[0])
	}, func() {
		cancel()
		<-returned
		eventuallyNoLockWaiters(t, pool)
	})
	if err == nil {
		t.Fatal("cancelled acknowledge reported success")
	}
	var state string
	var generation int64
	if err := pool.QueryRow(ctx, `SELECT state, generation FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state, &generation); err != nil {
		t.Fatal(err)
	}
	if state != "in_flight" || generation != markers[0].Generation+1 {
		t.Fatalf("state=%q generation=%d; want the claim untouched with the newer generation", state, generation)
	}
	if acknowledged, err := repo.Acknowledge(ctx, markers[0]); err != nil || acknowledged {
		t.Fatalf("retried acknowledge: acknowledged=%v err=%v", acknowledged, err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM fga_outbox WHERE id = $1`, markers[0].ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "pending" {
		t.Fatalf("state after retried acknowledge = %q; want pending", state)
	}
}
