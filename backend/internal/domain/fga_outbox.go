// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"
	"time"
)

// FGAOutboxMarker is a generation-guarded dirty marker for derived FGA state.
type FGAOutboxMarker struct {
	ID                int64
	MarkerKind        string
	ObjectType        string
	ObjectUID         string
	Relation          *string
	Username          *string
	DesiredOperation  string
	Generation        int64
	ClaimedGeneration *int64
	ClaimedAt         *time.Time
	Attempts          int
	LastError         *string
}

// FGAOutboxRepository claims and completes durable FGA sync markers.
type FGAOutboxRepository interface {
	EnqueueObject(ctx context.Context, objectType, objectUID, operation string) error
	EnqueueMembership(ctx context.Context, objectType, objectUID, relation, username string) error
	EnqueueMembershipRemoval(ctx context.Context, objectType, objectUID, relation, username string) error
	Claim(ctx context.Context, limit int) ([]FGAOutboxMarker, error)
	Acknowledge(ctx context.Context, marker FGAOutboxMarker) (bool, error)
	Retry(ctx context.Context, marker FGAOutboxMarker, nextAttemptAt time.Time, errText string) (bool, error)
	DeadLetter(ctx context.Context, marker FGAOutboxMarker, errText string) (bool, error)
}
