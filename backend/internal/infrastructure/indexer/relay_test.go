// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

type outboxStub struct {
	records       []domain.IndexOutboxRecord
	sent, retried string
}

func (s *outboxStub) Enqueue(context.Context, domain.IndexOutboxRecord) error { return nil }
func (s *outboxStub) Claim(context.Context, int) ([]domain.IndexOutboxRecord, error) {
	return s.records, nil
}
func (s *outboxStub) MarkSent(_ context.Context, id string) error  { s.sent = id; return nil }
func (s *outboxStub) MarkRetry(_ context.Context, id string) error { s.retried = id; return nil }

type publisherStub struct {
	subject string
	data    []byte
	err     error
}

func (s *publisherStub) Publish(subject string, data []byte) error {
	s.subject, s.data = subject, data
	return s.err
}

func TestRelayRunOncePublishesDelete(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", ObjectUID: "p1", Action: "deleted", Headers: json.RawMessage(`{"authorization":"Bearer x"}`)}}}
	publisher := &publisherStub{}
	if err := NewRelay(outbox, publisher, 1).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if publisher.subject != "lfx.index.mentorship_program" || outbox.sent != "1" {
		t.Fatalf("subject=%q sent=%q", publisher.subject, outbox.sent)
	}
	var envelope Envelope
	if err := json.Unmarshal(publisher.data, &envelope); err != nil {
		t.Fatal(err)
	}
	if string(envelope.Data) != `"p1"` {
		t.Fatalf("data=%s", envelope.Data)
	}
}

func TestRelayRunOnceRetriesPublishFailure(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}}
	if err := NewRelay(outbox, &publisherStub{err: errors.New("down")}, 1).RunOnce(context.Background()); err == nil || outbox.retried != "1" {
		t.Fatalf("err=%v retried=%q", err, outbox.retried)
	}
}
