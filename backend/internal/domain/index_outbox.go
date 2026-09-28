// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"
	"encoding/json"
	"time"
)

type IndexOutboxRecord struct {
	ID                string
	ObjectType        string
	ObjectUID         string
	Action            string
	Headers           json.RawMessage
	Data              json.RawMessage
	IndexingConfig    json.RawMessage
	Attempts          int
	Generation        int64
	ClaimedGeneration *int64
	ClaimedAt         *time.Time
	CreatedOn         time.Time
}

type IndexOutboxRepository interface {
	Enqueue(ctx context.Context, record IndexOutboxRecord) error
	Claim(ctx context.Context, limit int) ([]IndexOutboxRecord, error)
	MarkSent(ctx context.Context, record IndexOutboxRecord) (bool, error)
	MarkRetry(ctx context.Context, record IndexOutboxRecord) (acknowledged bool, deadLettered bool, err error)
}
