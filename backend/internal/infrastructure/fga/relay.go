// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"log/slog"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

var (
	relayClaimed       = expvar.NewInt("fga_relay_claimed")
	relayPublished     = expvar.NewInt("fga_relay_published")
	relayRetried       = expvar.NewInt("fga_relay_retried")
	relayAckFailures   = expvar.NewInt("fga_relay_ack_failures")
	relayClaimFailures = expvar.NewInt("fga_relay_claim_failures")
	relayDeadLettered  = expvar.NewInt("fga_relay_dead_lettered")
)

// Outbox is the persistence boundary used by Relay.
type Outbox interface {
	Claim(ctx context.Context, limit int) ([]domain.FGAOutboxMarker, error)
	Acknowledge(ctx context.Context, marker domain.FGAOutboxMarker) (bool, error)
	Retry(ctx context.Context, marker domain.FGAOutboxMarker, nextAttemptAt time.Time, errText string) (bool, error)
	DeadLetter(ctx context.Context, marker domain.FGAOutboxMarker, errText string) (bool, error)
}

// Builder reconstructs a fresh message from current source-of-truth state.
type Builder interface {
	Build(ctx context.Context, marker domain.FGAOutboxMarker) (Message, error)
}

// Publisher durably hands a message to the configured JetStream subject.
type Publisher interface {
	Publish(ctx context.Context, message Message) error
}

// Relay processes one bounded outbox batch in marker order.
type Relay struct {
	outbox      Outbox
	builder     Builder
	publisher   Publisher
	batchSize   int
	retryDelay  time.Duration
	maxAttempts int
	clock       func() time.Time
	logger      *slog.Logger
}

// NewRelay creates a bounded, sequential outbox relay.
func NewRelay(outbox Outbox, builder Builder, publisher Publisher, batchSize int, retryDelay time.Duration) *Relay {
	if batchSize <= 0 {
		batchSize = 50
	}
	if retryDelay <= 0 {
		retryDelay = time.Minute
	}
	return &Relay{
		outbox:      outbox,
		builder:     builder,
		publisher:   publisher,
		batchSize:   batchSize,
		retryDelay:  retryDelay,
		maxAttempts: 10,
		clock:       time.Now,
		logger:      slog.Default(),
	}
}

// SetMaxAttempts bounds retries before a marker is retained as dead-lettered.
func (r *Relay) SetMaxAttempts(maxAttempts int) {
	if maxAttempts > 0 {
		r.maxAttempts = maxAttempts
	}
}

// SetLogger configures the relay's operational error logger.
func (r *Relay) SetLogger(logger *slog.Logger) {
	if logger != nil {
		r.logger = logger
	}
}

// RunOnce processes the currently claimable batch. It does not stop the relay
// permanently when one marker fails; the marker is returned to the outbox.
func (r *Relay) RunOnce(ctx context.Context) error {
	markers, err := r.outbox.Claim(ctx, r.batchSize)
	if err != nil {
		relayClaimFailures.Add(1)
		return err
	}
	relayClaimed.Add(int64(len(markers)))
	var firstErr error
	for _, marker := range markers {
		message, buildErr := r.builder.Build(ctx, marker)
		if buildErr == nil {
			buildErr = r.publisher.Publish(ctx, message)
		}
		if buildErr != nil {
			firstErr = errors.Join(firstErr, buildErr)
			if retryErr := r.recordFailure(ctx, marker, buildErr); retryErr != nil {
				firstErr = errors.Join(firstErr, fmt.Errorf("retry marker %d: %w", marker.ID, retryErr))
			}
			continue
		}
		acknowledged, ackErr := r.outbox.Acknowledge(ctx, marker)
		if ackErr != nil {
			relayAckFailures.Add(1)
			if firstErr == nil {
				firstErr = ackErr
			}
		} else if acknowledged {
			relayPublished.Add(1)
		}
	}
	return firstErr
}

// recordFailure retries or dead-letters a failed marker and counts only transitions that persisted.
func (r *Relay) recordFailure(ctx context.Context, marker domain.FGAOutboxMarker, cause error) error {
	if marker.Attempts+1 >= r.maxAttempts {
		deadLettered, err := r.outbox.DeadLetter(ctx, marker, cause.Error())
		if err != nil {
			return err
		}
		if deadLettered {
			relayDeadLettered.Add(1)
			return nil
		}
		// A newer generation arrived while publishing; release it for a fresh attempt.
	}
	retried, err := r.outbox.Retry(ctx, marker, r.clock().Add(r.retryDelay), cause.Error())
	if err != nil {
		return err
	}
	if retried {
		relayRetried.Add(1)
	}
	return nil
}

// Run polls the outbox until the context is cancelled.
func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := r.RunOnce(ctx); err != nil {
			r.logger.ErrorContext(ctx, "FGA relay batch failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
