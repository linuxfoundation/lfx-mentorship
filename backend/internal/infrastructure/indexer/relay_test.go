// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

const testToken = "service-token"

type outboxStub struct {
	records               []domain.IndexOutboxRecord
	claimed               bool
	sent, retried         string
	markSentAcknowledged  bool
	markRetryAcknowledged bool
}

func (s *outboxStub) Enqueue(context.Context, domain.IndexOutboxRecord) error { return nil }
func (s *outboxStub) Claim(context.Context, int) ([]domain.IndexOutboxRecord, error) {
	s.claimed = true
	return s.records, nil
}
func (s *outboxStub) MarkSent(_ context.Context, record domain.IndexOutboxRecord) (bool, error) {
	s.sent = record.ID
	if !s.markSentAcknowledged {
		return false, nil
	}
	return true, nil
}
func (s *outboxStub) MarkRetry(_ context.Context, record domain.IndexOutboxRecord) (bool, bool, error) {
	s.retried = record.ID
	if !s.markRetryAcknowledged {
		return false, false, nil
	}
	return true, false, nil
}

type publisherStub struct {
	subject string
	data    []byte
	reply   string
	err     error
}

func (s *publisherStub) RequestWithContext(_ context.Context, subject string, data []byte) (*nats.Msg, error) {
	s.subject, s.data = subject, data
	if s.err != nil {
		return nil, s.err
	}
	reply := s.reply
	if reply == "" {
		reply = indexerAck
	}
	return &nats.Msg{Data: []byte(reply)}, nil
}

func TestRelayRunOncePublishesDelete(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", ObjectUID: "p1", Action: "deleted", Headers: json.RawMessage(`{"authorization":"Bearer x"}`)}}, markSentAcknowledged: true, markRetryAcknowledged: true}
	publisher := &publisherStub{}
	if err := NewRelay(outbox, publisher, 1, testToken).RunOnce(context.Background()); err != nil {
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
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markSentAcknowledged: true, markRetryAcknowledged: true}
	if err := NewRelay(outbox, &publisherStub{err: errors.New("down")}, 1, testToken).RunOnce(context.Background()); err == nil || outbox.retried != "1" {
		t.Fatalf("err=%v retried=%q", err, outbox.retried)
	}
}

func TestRelayRunOnceRetriesWhenIndexerRejectsMessage(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markSentAcknowledged: true, markRetryAcknowledged: true}
	err := NewRelay(outbox, &publisherStub{reply: "ERROR: error processing indexing message"}, 1, testToken).RunOnce(context.Background())
	if err == nil || outbox.retried != "1" || outbox.sent != "" {
		t.Fatalf("err=%v retried=%q sent=%q; want retry and no mark-sent", err, outbox.retried, outbox.sent)
	}
}

func TestRelayRunOnceRetriesWhenIndexerHasNoResponders(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markSentAcknowledged: true, markRetryAcknowledged: true}
	err := NewRelay(outbox, &publisherStub{err: nats.ErrNoResponders}, 1, testToken).RunOnce(context.Background())
	if !errors.Is(err, nats.ErrNoResponders) || outbox.retried != "1" || outbox.sent != "" {
		t.Fatalf("err=%v retried=%q sent=%q; want a retried record", err, outbox.retried, outbox.sent)
	}
}

func TestRelayRunOnceIdlesWithoutServiceToken(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markSentAcknowledged: true}
	publisher := &publisherStub{}
	if err := NewRelay(outbox, publisher, 1, "  ").RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if outbox.claimed || publisher.subject != "" {
		t.Fatalf("claimed=%v published=%q; want rows left pending", outbox.claimed, publisher.subject)
	}
}

func TestRelayStampsServiceTokenOverStoredHeaderAndDropsStoredActor(t *testing.T) {
	outbox := &outboxStub{
		records: []domain.IndexOutboxRecord{{
			ID:         "1",
			ObjectType: "mentorship_program",
			Action:     "updated",
			Headers:    json.RawMessage(`{"authorization":"present","x-on-behalf-of":"alice"}`),
		}},
		markSentAcknowledged:  true,
		markRetryAcknowledged: true,
	}
	publisher := &publisherStub{}
	relay := NewRelay(outbox, publisher, 1, testToken)
	if err := relay.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var envelope Envelope
	if err := json.Unmarshal(publisher.data, &envelope); err != nil {
		t.Fatal(err)
	}
	var headers map[string]string
	if err := json.Unmarshal(envelope.Headers, &headers); err != nil {
		t.Fatal(err)
	}
	if headers["authorization"] != "Bearer "+testToken {
		t.Fatalf("headers=%v", headers)
	}
	if _, ok := headers["x-on-behalf-of"]; ok {
		t.Fatalf("stored client actor header was republished: %v", headers)
	}
}

func TestRelayDropsStoredActorAndKeepsPrefixedToken(t *testing.T) {
	outbox := &outboxStub{
		records: []domain.IndexOutboxRecord{{
			ID:         "1",
			ObjectType: "mentorship_program",
			Action:     "updated",
			Headers:    json.RawMessage(`{"x-on-behalf-of":"alice"}`),
		}},
		markSentAcknowledged: true,
	}
	publisher := &publisherStub{}
	if err := NewRelay(outbox, publisher, 1, "Bearer "+testToken).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var envelope Envelope
	if err := json.Unmarshal(publisher.data, &envelope); err != nil {
		t.Fatal(err)
	}
	var headers map[string]string
	if err := json.Unmarshal(envelope.Headers, &headers); err != nil {
		t.Fatal(err)
	}
	if _, ok := headers["x-on-behalf-of"]; ok {
		t.Fatalf("stored client actor header was republished: %v", headers)
	}
	if headers["authorization"] != "Bearer "+testToken {
		t.Fatalf("authorization=%q; want an already-prefixed token left unchanged", headers["authorization"])
	}
}

func TestRelayRunOnceFailsWhenMarkSentIsNotAcknowledged(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markRetryAcknowledged: true}
	err := NewRelay(outbox, &publisherStub{}, 1, testToken).RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected acknowledgement error")
	}
}

func TestRelayRunOnceCountsEveryMarkSentAcknowledgementFailure(t *testing.T) {
	start := indexRelayAckFailures.Value()
	outbox := &outboxStub{
		records: []domain.IndexOutboxRecord{
			{ID: "1", ObjectType: "mentorship_program", Action: "updated"},
			{ID: "2", ObjectType: "mentorship_program", Action: "updated"},
		},
		markRetryAcknowledged: true,
	}

	if err := NewRelay(outbox, &publisherStub{}, 2, testToken).RunOnce(context.Background()); err == nil {
		t.Fatal("expected acknowledgement error")
	}
	if got := indexRelayAckFailures.Value() - start; got != 2 {
		t.Fatalf("ack failure count delta=%d, want 2", got)
	}
}

func TestRelayRunOnceFailsWhenMarkRetryIsNotAcknowledged(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markSentAcknowledged: true}
	err := NewRelay(outbox, &publisherStub{err: errors.New("down")}, 1, testToken).RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected retry acknowledgement error")
	}
}
