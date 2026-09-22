// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type jetStreamStub struct {
	subject string
	data    []byte
}

func (s *jetStreamStub) PublishMsg(_ context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	s.subject = msg.Subject
	s.data = msg.Data
	return &jetstream.PubAck{}, nil
}

func TestJetStreamPublisherPublishesToContractSubject(t *testing.T) {
	stub := &jetStreamStub{}
	publisher := NewJetStreamPublisher(stub)

	message := Message{
		ObjectType: "mentorship_program",
		Operation:  memberPutOperation,
		Data:       MemberData{UID: "program-1", Username: "mentor-1", Relations: []string{"mentor"}},
	}
	if err := publisher.Publish(context.Background(), message); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if stub.subject != memberPutSubject {
		t.Fatalf("subject = %q, want %q", stub.subject, memberPutSubject)
	}
	if len(stub.data) == 0 {
		t.Fatal("expected serialized message")
	}
}

func TestJetStreamPublisherRejectsUnsupportedOperation(t *testing.T) {
	publisher := NewJetStreamPublisher(&jetStreamStub{})
	if err := publisher.Publish(context.Background(), Message{ObjectType: "mentorship_program", Operation: "invalid", Data: struct{}{}}); err == nil {
		t.Fatal("expected unsupported operation error")
	}
}
