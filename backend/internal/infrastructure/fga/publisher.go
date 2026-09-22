// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	updateAccessSubject = "lfx.fga-sync.update_access"
	deleteAccessSubject = "lfx.fga-sync.delete_access"
	memberPutSubject    = "lfx.fga-sync.member_put"
	memberRemoveSubject = "lfx.fga-sync.member_remove"
)

// JetStreamClient is the subset of JetStream needed by the publisher.
type JetStreamClient interface {
	PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// JetStreamPublisher durably publishes FGA messages to the shared stream.
type JetStreamPublisher struct {
	js JetStreamClient
}

// NewJetStreamPublisher creates a publisher backed by JetStream.
func NewJetStreamPublisher(js JetStreamClient) *JetStreamPublisher {
	return &JetStreamPublisher{js: js}
}

// Publish serializes and publishes one asynchronous FGA mutation.
func (p *JetStreamPublisher) Publish(ctx context.Context, message Message) error {
	if p == nil || p.js == nil {
		return errors.New("FGA JetStream publisher is not configured")
	}
	subject, err := subjectForOperation(message.Operation)
	if err != nil {
		return err
	}
	if message.ObjectType == "" {
		return errors.New("FGA object type is required")
	}
	if message.Data == nil {
		return errors.New("FGA message data is required")
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal FGA message: %w", err)
	}
	if _, err := p.js.PublishMsg(ctx, &nats.Msg{Subject: subject, Data: payload}); err != nil {
		return fmt.Errorf("publish FGA message to %s: %w", subject, err)
	}
	return nil
}

func subjectForOperation(operation string) (string, error) {
	switch operation {
	case updateAccessOperation:
		return updateAccessSubject, nil
	case deleteAccessOperation:
		return deleteAccessSubject, nil
	case memberPutOperation:
		return memberPutSubject, nil
	case memberRemoveOperation:
		return memberRemoveSubject, nil
	default:
		return "", fmt.Errorf("unsupported FGA operation %q", operation)
	}
}
