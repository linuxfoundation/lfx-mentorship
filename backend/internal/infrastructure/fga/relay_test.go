// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

type relayOutboxStub struct {
	markers         []domain.FGAOutboxMarker
	acknowledged    []int64
	retried         []int64
	nextAttemptAt   time.Time
	retryErr        error
	deadLettered    []int64
	retryLost       bool
	deadLetterStale bool
}

func (s *relayOutboxStub) Claim(context.Context, int) ([]domain.FGAOutboxMarker, error) {
	return s.markers, nil
}
func (s *relayOutboxStub) Acknowledge(_ context.Context, marker domain.FGAOutboxMarker) (bool, error) {
	s.acknowledged = append(s.acknowledged, marker.ID)
	return true, nil
}
func (s *relayOutboxStub) Retry(_ context.Context, marker domain.FGAOutboxMarker, nextAttemptAt time.Time, _ string) (bool, error) {
	s.retried = append(s.retried, marker.ID)
	s.nextAttemptAt = nextAttemptAt
	return s.retryErr == nil && !s.retryLost, s.retryErr
}
func (s *relayOutboxStub) DeadLetter(_ context.Context, marker domain.FGAOutboxMarker, _ string) (bool, error) {
	s.deadLettered = append(s.deadLettered, marker.ID)
	return s.retryErr == nil && !s.deadLetterStale, s.retryErr
}

type relayBuilderStub struct {
	fail bool
}

func (s relayBuilderStub) Build(context.Context, domain.FGAOutboxMarker) (Message, error) {
	if s.fail {
		return Message{}, errors.New("build failed")
	}
	return Message{ObjectType: "mentorship_program", Operation: "update_access"}, nil
}

type relayPublisherStub struct {
	published int
	fail      bool
}

func (s *relayPublisherStub) Publish(context.Context, Message) error {
	s.published++
	if s.fail {
		return errors.New("publish failed")
	}
	return nil
}

func TestRelayRunOnceAcknowledgesSuccessfulMessages(t *testing.T) {
	outbox := &relayOutboxStub{markers: []domain.FGAOutboxMarker{{ID: 1}}}
	publisher := &relayPublisherStub{}
	relay := NewRelay(outbox, relayBuilderStub{}, publisher, 10, time.Minute)

	if err := relay.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if publisher.published != 1 || len(outbox.acknowledged) != 1 || len(outbox.retried) != 0 {
		t.Fatalf("unexpected relay result: published=%d acknowledged=%v retried=%v", publisher.published, outbox.acknowledged, outbox.retried)
	}
}

func TestRelayRunOnceRetriesBuildFailures(t *testing.T) {
	outbox := &relayOutboxStub{markers: []domain.FGAOutboxMarker{{ID: 2}}}
	relay := NewRelay(outbox, relayBuilderStub{fail: true}, &relayPublisherStub{}, 10, time.Minute)

	if err := relay.RunOnce(context.Background()); err == nil {
		t.Fatal("expected build error")
	}
	if len(outbox.retried) != 1 || len(outbox.acknowledged) != 0 {
		t.Fatalf("unexpected retry result: acknowledged=%v retried=%v", outbox.acknowledged, outbox.retried)
	}
}

func TestRelayRunOnceReturnsRetryPersistenceFailure(t *testing.T) {
	retryErr := errors.New("outbox unavailable")
	outbox := &relayOutboxStub{
		markers:  []domain.FGAOutboxMarker{{ID: 3}},
		retryErr: retryErr,
	}
	relay := NewRelay(outbox, relayBuilderStub{fail: true}, &relayPublisherStub{}, 10, time.Minute)

	err := relay.RunOnce(context.Background())
	if !errors.Is(err, retryErr) {
		t.Fatalf("expected retry persistence error, got %v", err)
	}
}

func TestRelayRunOnceDeadLettersAfterMaxAttempts(t *testing.T) {
	outbox := &relayOutboxStub{markers: []domain.FGAOutboxMarker{{ID: 4, Attempts: 2}}}
	relay := NewRelay(outbox, relayBuilderStub{fail: true}, &relayPublisherStub{}, 10, time.Minute)
	relay.SetMaxAttempts(3)

	if err := relay.RunOnce(context.Background()); err == nil {
		t.Fatal("expected build error")
	}
	if len(outbox.deadLettered) != 1 || len(outbox.retried) != 0 {
		t.Fatalf("unexpected dead-letter result: dead_lettered=%v retried=%v", outbox.deadLettered, outbox.retried)
	}
}

func TestRelayRunOnceRetriesNewerGenerationInsteadOfCountingDeadLetter(t *testing.T) {
	outbox := &relayOutboxStub{markers: []domain.FGAOutboxMarker{{ID: 5, Attempts: 2}}, deadLetterStale: true}
	relay := NewRelay(outbox, relayBuilderStub{fail: true}, &relayPublisherStub{}, 10, time.Minute)
	relay.SetMaxAttempts(3)
	deadLetteredBefore, retriedBefore := relayDeadLettered.Value(), relayRetried.Value()

	if err := relay.RunOnce(context.Background()); err == nil {
		t.Fatal("expected build error")
	}
	if len(outbox.deadLettered) != 1 || len(outbox.retried) != 1 {
		t.Fatalf("dead_lettered=%v retried=%v; want one attempt each", outbox.deadLettered, outbox.retried)
	}
	if got := relayDeadLettered.Value() - deadLetteredBefore; got != 0 {
		t.Fatalf("dead-letter metric delta=%d; want 0", got)
	}
	if got := relayRetried.Value() - retriedBefore; got != 1 {
		t.Fatalf("retry metric delta=%d; want 1", got)
	}
}

func TestRelayRunOnceDoesNotCountLostRetry(t *testing.T) {
	outbox := &relayOutboxStub{markers: []domain.FGAOutboxMarker{{ID: 6}}, retryLost: true}
	relay := NewRelay(outbox, relayBuilderStub{fail: true}, &relayPublisherStub{}, 10, time.Minute)
	retriedBefore := relayRetried.Value()

	if err := relay.RunOnce(context.Background()); err == nil {
		t.Fatal("expected build error")
	}
	if got := relayRetried.Value() - retriedBefore; got != 0 {
		t.Fatalf("retry metric delta=%d; want 0", got)
	}
}
