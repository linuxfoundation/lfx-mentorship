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
	provider      authorizationProvider
}

func (r *Relay) SetLogger(logger *slog.Logger) {
	r.logger = logger
}

func (r *Relay) SetAuthorization(authorization string) {
	r.authorization = authorization
}

func (r *Relay) SetAuthorizationProvider(provider authorizationProvider) {
	r.provider = provider
}

type publisher interface {
	Publish(subject string, data []byte) error
}

type authorizationProvider interface {
	Authorization(context.Context) (string, error)
}

type flusher interface {
	Flush() error
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
	authorization := r.authorization
	if r.provider != nil {
		authorization, err = r.provider.Authorization(ctx)
		if err != nil {
			for _, record := range records {
				_, _ = r.outbox.MarkRetry(ctx, record)
			}
			return fmt.Errorf("obtain index authorization: %w", err)
		}
	}
	var firstErr error
	for _, record := range records {
		headers := record.Headers
		var recordErr error
		if authorization != "" {
			var headerValues map[string]string
			if len(headers) > 0 {
				if err := json.Unmarshal(headers, &headerValues); err != nil {
					recordErr = fmt.Errorf("decode index headers for record %s: %w", record.ID, err)
				}
			} else {
				headerValues = map[string]string{}
			}
			if recordErr == nil {
				headerValues["authorization"] = authorization
				headers, recordErr = json.Marshal(headerValues)
				if recordErr != nil {
					recordErr = fmt.Errorf("marshal index authorization for record %s: %w", record.ID, recordErr)
				}
			}
		}
		if recordErr == nil {
			envelope := Envelope{Action: record.Action, Headers: headers, Data: record.Data, IndexingConfig: record.IndexingConfig}
			if record.Action == "deleted" {
				envelope.Data, recordErr = json.Marshal(record.ObjectUID)
			}
			var payload []byte
			if recordErr == nil {
				payload, recordErr = json.Marshal(envelope)
			}
			if recordErr == nil {
				recordErr = r.conn.Publish("lfx.index."+record.ObjectType, payload)
			}
			if recordErr == nil {
				if publisher, ok := r.conn.(flusher); ok {
					recordErr = publisher.Flush()
				}
			}
		}
		if recordErr != nil {
			if acknowledged, retryErr := r.outbox.MarkRetry(ctx, record); retryErr != nil {
				recordErr = fmt.Errorf("mark retry for index record %s: %w", record.ID, retryErr)
			} else if !acknowledged {
				recordErr = fmt.Errorf("mark retry for index record %s was not acknowledged", record.ID)
			}
			if firstErr == nil {
				firstErr = recordErr
			}
			continue
		}
		acknowledged, markErr := r.outbox.MarkSent(ctx, record)
		if markErr != nil {
			recordErr = markErr
		} else if !acknowledged {
			recordErr = fmt.Errorf("mark sent for index record %s was not acknowledged", record.ID)
		}
		if recordErr != nil && firstErr == nil {
			firstErr = recordErr
		}
	}
	return firstErr
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
