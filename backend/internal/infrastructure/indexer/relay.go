// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

type Envelope struct {
	Action         string          `json:"action"`
	Headers        json.RawMessage `json:"headers"`
	Data           json.RawMessage `json:"data"`
	IndexingConfig json.RawMessage `json:"indexing_config,omitempty"`
}

type Relay struct {
	outbox domain.IndexOutboxRepository
	conn   publisher
	batch  int
}

type publisher interface {
	Publish(subject string, data []byte) error
}

func NewRelay(outbox domain.IndexOutboxRepository, conn publisher, batch int) *Relay {
	if batch <= 0 {
		batch = 50
	}
	return &Relay{outbox: outbox, conn: conn, batch: batch}
}

func (r *Relay) RunOnce(ctx context.Context) error {
	records, err := r.outbox.Claim(ctx, r.batch)
	if err != nil {
		return err
	}
	for _, record := range records {
		envelope := Envelope{Action: record.Action, Headers: record.Headers, Data: record.Data, IndexingConfig: record.IndexingConfig}
		if record.Action == "deleted" {
			envelope.Data, _ = json.Marshal(record.ObjectUID)
		}
		payload, marshalErr := json.Marshal(envelope)
		if marshalErr == nil {
			marshalErr = r.conn.Publish("lfx.index."+record.ObjectType, payload)
		}
		if marshalErr != nil {
			_ = r.outbox.MarkRetry(ctx, record.ID)
			return fmt.Errorf("publish index record %s: %w", record.ID, marshalErr)
		}
		if err := r.outbox.MarkSent(ctx, record.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = r.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
