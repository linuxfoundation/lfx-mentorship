// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"
	"encoding/json"
	"time"
)

type IndexOutboxRecord struct {
	ID             string
	ObjectType     string
	ObjectUID      string
	Action         string
	Headers        json.RawMessage
	Data           json.RawMessage
	IndexingConfig json.RawMessage
	Attempts       int
	CreatedOn      time.Time
}

type IndexOutboxRepository interface {
	Enqueue(ctx context.Context, record IndexOutboxRecord) error
	Claim(ctx context.Context, limit int) ([]IndexOutboxRecord, error)
	MarkSent(ctx context.Context, id string) error
	MarkRetry(ctx context.Context, id string) error
}
