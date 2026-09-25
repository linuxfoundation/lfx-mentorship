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
	records               []domain.IndexOutboxRecord
	sent, retried         string
	markSentAcknowledged  bool
	markRetryAcknowledged bool
}

func (s *outboxStub) Enqueue(context.Context, domain.IndexOutboxRecord) error { return nil }
func (s *outboxStub) Claim(context.Context, int) ([]domain.IndexOutboxRecord, error) {
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
	err     error
}

func (s *publisherStub) Publish(subject string, data []byte) error {
	s.subject, s.data = subject, data
	return s.err
}

func TestRelayRunOncePublishesDelete(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", ObjectUID: "p1", Action: "deleted", Headers: json.RawMessage(`{"authorization":"Bearer x"}`)}}, markSentAcknowledged: true, markRetryAcknowledged: true}
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
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markSentAcknowledged: true, markRetryAcknowledged: true}
	if err := NewRelay(outbox, &publisherStub{err: errors.New("down")}, 1).RunOnce(context.Background()); err == nil || outbox.retried != "1" {
		t.Fatalf("err=%v retried=%q", err, outbox.retried)
	}
}

func TestRelaySetAuthorizationOverridesStoredHeaderAndDropsStoredActor(t *testing.T) {
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
	relay := NewRelay(outbox, publisher, 1)
	relay.SetAuthorization("Bearer machine-token")
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
	if headers["authorization"] != "Bearer machine-token" {
		t.Fatalf("headers=%v", headers)
	}
	if _, ok := headers["x-on-behalf-of"]; ok {
		t.Fatalf("stored client actor header was republished: %v", headers)
	}
}

func TestRelayDropsStoredActorWithoutAuthorization(t *testing.T) {
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
	if err := NewRelay(outbox, publisher, 1).RunOnce(context.Background()); err != nil {
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
}

func TestRelayRunOnceFailsWhenMarkSentIsNotAcknowledged(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markRetryAcknowledged: true}
	err := NewRelay(outbox, &publisherStub{}, 1).RunOnce(context.Background())
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

	if err := NewRelay(outbox, &publisherStub{}, 2).RunOnce(context.Background()); err == nil {
		t.Fatal("expected acknowledgement error")
	}
	if got := indexRelayAckFailures.Value() - start; got != 2 {
		t.Fatalf("ack failure count delta=%d, want 2", got)
	}
}

func TestRelayRunOnceFailsWhenMarkRetryIsNotAcknowledged(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markSentAcknowledged: true}
	err := NewRelay(outbox, &publisherStub{err: errors.New("down")}, 1).RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected retry acknowledgement error")
	}
}

type failingAuthorizationProvider struct{}

func (failingAuthorizationProvider) Authorization(context.Context) (string, error) {
	return "", errors.New("token endpoint down")
}

func TestRelayRunOnceRetriesClaimedRecordsWhenAuthorizationFails(t *testing.T) {
	start := indexRelayRetried.Value()
	outbox := &outboxStub{
		records: []domain.IndexOutboxRecord{
			{ID: "1", ObjectType: "mentorship_program", Action: "updated"},
			{ID: "2", ObjectType: "mentorship_program", Action: "updated"},
		},
		markRetryAcknowledged: true,
	}
	publisher := &publisherStub{}
	relay := NewRelay(outbox, publisher, 2)
	relay.SetAuthorizationProvider(failingAuthorizationProvider{})

	if err := relay.RunOnce(context.Background()); err == nil {
		t.Fatal("expected authorization error")
	}
	if publisher.subject != "" || outbox.sent != "" {
		t.Fatalf("published subject=%q sent=%q; want nothing published", publisher.subject, outbox.sent)
	}
	if got := indexRelayRetried.Value() - start; got != 2 {
		t.Fatalf("retried count delta=%d, want 2", got)
	}
}

func TestRelayRunOnceDoesNotCountUnacknowledgedRetriesWhenAuthorizationFails(t *testing.T) {
	start := indexRelayRetried.Value()
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}}
	relay := NewRelay(outbox, &publisherStub{}, 1)
	relay.SetAuthorizationProvider(failingAuthorizationProvider{})

	if err := relay.RunOnce(context.Background()); err == nil {
		t.Fatal("expected authorization error")
	}
	if outbox.retried != "1" {
		t.Fatalf("retried=%q, want 1", outbox.retried)
	}
	if got := indexRelayRetried.Value() - start; got != 0 {
		t.Fatalf("retried count delta=%d, want 0 for an unacknowledged retry", got)
	}
}
