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
func (s *outboxStub) MarkRetry(_ context.Context, record domain.IndexOutboxRecord) (bool, error) {
	s.retried = record.ID
	if !s.markRetryAcknowledged {
		return false, nil
	}
	return true, nil
}

type publisherStub struct {
	subject string
	data    []byte
	err     error
}

type authorizationProviderStub struct {
	err error
}

func (s authorizationProviderStub) Authorization(context.Context) (string, error) {
	return "", s.err
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

func TestRelayRunOnceFailsWhenMarkSentIsNotAcknowledged(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markRetryAcknowledged: true}
	err := NewRelay(outbox, &publisherStub{}, 1).RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected acknowledgement error")
	}
}

func TestRelayRunOnceFailsWhenMarkRetryIsNotAcknowledged(t *testing.T) {
	outbox := &outboxStub{records: []domain.IndexOutboxRecord{{ID: "1", ObjectType: "mentorship_program", Action: "updated"}}, markSentAcknowledged: true}
	err := NewRelay(outbox, &publisherStub{err: errors.New("down")}, 1).RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected retry acknowledgement error")
	}
}

func TestRelayRunOnceRetriesClaimedRecordsWhenAuthorizationFails(t *testing.T) {
	outbox := &outboxStub{
		records:               []domain.IndexOutboxRecord{{ID: "1"}, {ID: "2"}},
		markRetryAcknowledged: true,
	}
	relay := NewRelay(outbox, &publisherStub{}, 2)
	relay.SetAuthorizationProvider(authorizationProviderStub{err: errors.New("token endpoint down")})
	if err := relay.RunOnce(context.Background()); err == nil {
		t.Fatal("expected authorization error")
	}
	if outbox.retried != "2" {
		t.Fatalf("last retried record=%q; want 2", outbox.retried)
	}
}
