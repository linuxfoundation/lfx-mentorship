// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	outbox        domain.IndexOutboxRepository
	conn          publisher
	batch         int
	logger        *slog.Logger
	authorization string
}

func (r *Relay) SetLogger(logger *slog.Logger) {
	r.logger = logger
}

func (r *Relay) SetAuthorization(authorization string) {
	r.authorization = authorization
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
		headers := record.Headers
		if r.authorization != "" {
			var headerValues map[string]string
			if len(headers) > 0 {
				if err := json.Unmarshal(headers, &headerValues); err != nil {
					return fmt.Errorf("decode index headers for record %s: %w", record.ID, err)
				}
			} else {
				headerValues = map[string]string{}
			}
			headerValues["authorization"] = r.authorization
			headers, err = json.Marshal(headerValues)
			if err != nil {
				return fmt.Errorf("marshal index authorization for record %s: %w", record.ID, err)
			}
		}
		envelope := Envelope{Action: record.Action, Headers: headers, Data: record.Data, IndexingConfig: record.IndexingConfig}
		if record.Action == "deleted" {
			envelope.Data, _ = json.Marshal(record.ObjectUID)
		}
		payload, marshalErr := json.Marshal(envelope)
		if marshalErr == nil {
			marshalErr = r.conn.Publish("lfx.index."+record.ObjectType, payload)
		}
		if marshalErr != nil {
			if acknowledged, retryErr := r.outbox.MarkRetry(ctx, record); retryErr != nil {
				return fmt.Errorf("mark retry for index record %s: %w", record.ID, retryErr)
			} else if !acknowledged {
				return fmt.Errorf("mark retry for index record %s was not acknowledged", record.ID)
			}
			return fmt.Errorf("publish index record %s: %w", record.ID, marshalErr)
		}
		acknowledged, err := r.outbox.MarkSent(ctx, record)
		if err != nil {
			return err
		}
		if !acknowledged {
			return fmt.Errorf("mark sent for index record %s was not acknowledged", record.ID)
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
		if err := r.RunOnce(ctx); err != nil && r.logger != nil {
			r.logger.Error("index outbox relay failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
