// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package indexer

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

const defaultPublishTimeout = 10 * time.Second

var (
	indexRelayClaimed       = expvar.NewInt("index_relay_claimed")
	indexRelayPublished     = expvar.NewInt("index_relay_published")
	indexRelayRetried       = expvar.NewInt("index_relay_retried")
	indexRelayClaimFailures = expvar.NewInt("index_relay_claim_failures")
	indexRelayAckFailures   = expvar.NewInt("index_relay_ack_failures")
	indexRelayDeadLettered  = expvar.NewInt("index_relay_dead_lettered")
)

type Envelope struct {
	Action         string          `json:"action"`
	Headers        json.RawMessage `json:"headers"`
	Data           json.RawMessage `json:"data"`
	IndexingConfig json.RawMessage `json:"indexing_config,omitempty"`
}

type Relay struct {
	outbox         domain.IndexOutboxRepository
	conn           publisher
	batch          int
	publishTimeout time.Duration
	logger         *slog.Logger
	authorization  string
	warnOnce       sync.Once
}

func (r *Relay) SetLogger(logger *slog.Logger) {
	if logger != nil {
		r.logger = logger
	}
}

type publisher interface {
	PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// NewRelay stamps serviceToken on every message; without one it idles, since the indexer drops unauthenticated messages.
func NewRelay(outbox domain.IndexOutboxRepository, conn publisher, batch int, serviceToken string) *Relay {
	if batch <= 0 {
		batch = 50
	}
	return &Relay{
		outbox:         outbox,
		conn:           conn,
		batch:          batch,
		publishTimeout: defaultPublishTimeout,
		logger:         slog.Default(),
		authorization:  bearer(serviceToken),
	}
}

func bearer(token string) string {
	token = strings.TrimSpace(token)
	if token == "" || strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return token
	}
	return "Bearer " + token
}

func (r *Relay) RunOnce(ctx context.Context) error {
	if r.authorization == "" {
		r.warnOnce.Do(func() {
			r.logger.WarnContext(ctx, "index relay is idle: INDEXER_SERVICE_TOKEN is not set; outbox rows stay pending")
		})
		return nil
	}
	records, err := r.outbox.Claim(ctx, r.batch)
	if err != nil {
		indexRelayClaimFailures.Add(1)
		return err
	}
	indexRelayClaimed.Add(int64(len(records)))
	var firstErr error
	for _, record := range records {
		headers := record.Headers
		var recordErr error
		var headerValues map[string]string
		if len(headers) > 0 {
			if err := json.Unmarshal(headers, &headerValues); err != nil {
				recordErr = fmt.Errorf("decode index headers for record %s: %w", record.ID, err)
			}
		}
		if recordErr == nil {
			// Rows stored before enqueue-time sanitization may still carry client-supplied actor headers.
			headerValues = domain.SanitizedIndexHeaders(headerValues)
			headerValues["authorization"] = r.authorization
			headers, recordErr = json.Marshal(headerValues)
			if recordErr != nil {
				recordErr = fmt.Errorf("marshal index headers for record %s: %w", record.ID, recordErr)
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
				recordErr = r.publishAndConfirm(ctx, "lfx.index."+record.ObjectType, payload)
				if recordErr != nil {
					recordErr = fmt.Errorf("index record %s: %w", record.ID, recordErr)
				}
			}
		}
		if recordErr != nil {
			if acknowledged, deadLettered, retryErr := r.outbox.MarkRetry(ctx, record); retryErr != nil {
				recordErr = fmt.Errorf("mark retry for index record %s: %w", record.ID, retryErr)
			} else if !acknowledged {
				recordErr = fmt.Errorf("mark retry for index record %s was not acknowledged", record.ID)
			} else {
				indexRelayRetried.Add(1)
				if deadLettered {
					indexRelayDeadLettered.Add(1)
				}
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
		if recordErr != nil {
			indexRelayAckFailures.Add(1)
			if firstErr == nil {
				firstErr = recordErr
			}
		} else {
			indexRelayPublished.Add(1)
		}
	}
	return firstErr
}

// publishAndConfirm counts only a JetStream publish acknowledgement as delivery.
func (r *Relay) publishAndConfirm(ctx context.Context, subject string, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, r.publishTimeout)
	defer cancel()
	if _, err := r.conn.PublishMsg(ctx, &nats.Msg{Subject: subject, Data: payload}); err != nil {
		return fmt.Errorf("indexer publish: %w", err)
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
		if err := r.RunOnce(ctx); err != nil {
			r.logger.ErrorContext(ctx, "index outbox relay failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
