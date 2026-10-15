// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package objectstore

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

var (
	objectDeletionDeleted      = expvar.NewInt("object_deletion_deleted")
	objectDeletionSkipped      = expvar.NewInt("object_deletion_skipped")
	objectDeletionRetried      = expvar.NewInt("object_deletion_retried")
	objectDeletionDeadLettered = expvar.NewInt("object_deletion_dead_lettered")
)

type objectDeleter interface {
	DeleteObject(ctx context.Context, key string) error
}

// RelayBucket is one bucket the relay deletes from.
type RelayBucket struct {
	Store objectDeleter
	// CDNURLPrefix is set for the CDN-fronted bucket, whose locators are URLs.
	CDNURLPrefix string
}

// Relay drains the object_deletions queue.
type Relay struct {
	queue   domain.ObjectDeletionRepository
	buckets map[domain.ObjectBucket]RelayBucket
	// claimable lists the configured buckets; entries for any other bucket wait until it is configured.
	claimable []domain.ObjectBucket
	batch     int
	logger    *slog.Logger
}

// NewRelay returns a Relay over the configured buckets.
func NewRelay(queue domain.ObjectDeletionRepository, buckets map[domain.ObjectBucket]RelayBucket, batch int, logger *slog.Logger) *Relay {
	if batch <= 0 {
		batch = 50
	}
	claimable := slices.Sorted(maps.Keys(buckets))
	return &Relay{queue: queue, buckets: buckets, claimable: claimable, batch: batch, logger: logger}
}

// RunOnce claims and processes one batch of due entries.
func (r *Relay) RunOnce(ctx context.Context) error {
	if len(r.claimable) == 0 {
		return nil
	}
	entries, err := r.queue.Claim(ctx, r.claimable, r.batch)
	if err != nil {
		return err
	}
	var firstErr error
	for _, entry := range entries {
		if err := r.process(ctx, entry); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *Relay) process(ctx context.Context, entry domain.ObjectDeletion) error {
	err := r.delete(ctx, entry)
	if err == nil {
		return r.queue.MarkDone(ctx, entry)
	}
	deadLettered, retryErr := r.queue.MarkRetry(ctx, entry, err)
	if retryErr != nil {
		return errors.Join(err, retryErr)
	}
	objectDeletionRetried.Add(1)
	if deadLettered {
		objectDeletionDeadLettered.Add(1)
		// A dead-lettered entry is a file, possibly PII, left in the bucket.
		r.logger.ErrorContext(ctx, "object deletion dead-lettered; the object is still stored",
			"entry_id", entry.ID, "bucket", entry.Bucket, "error", err)
	}
	return err
}

// delete removes the entry's object unless it is still referenced or was never minted here.
func (r *Relay) delete(ctx context.Context, entry domain.ObjectDeletion) error {
	referenced, err := r.queue.IsReferenced(ctx, entry.Locator)
	if err != nil {
		return err
	}
	if referenced {
		objectDeletionSkipped.Add(1)
		return nil
	}
	bucket, configured := r.buckets[entry.Bucket]
	if !configured {
		return fmt.Errorf("bucket %q is not configured", entry.Bucket)
	}
	key, minted := domain.ObjectKeyFromLocator(entry.Bucket, entry.Locator, bucket.CDNURLPrefix)
	if !minted {
		objectDeletionSkipped.Add(1)
		return nil
	}
	if err := bucket.Store.DeleteObject(ctx, key); err != nil {
		return err
	}
	objectDeletionDeleted.Add(1)
	return nil
}

// Run processes the queue every interval until ctx is cancelled.
func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := r.RunOnce(ctx); err != nil && ctx.Err() == nil {
			r.logger.ErrorContext(ctx, "object deletion relay failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
