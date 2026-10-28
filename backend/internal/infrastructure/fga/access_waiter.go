// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// accessCheckSubject is fga-sync's request/reply batch check. It answers from
// OpenFGA, the store Heimdall's openfga_check authorizer reads, through fga-sync's
// cache (invalidated on every tuple write) and OpenFGA's check-query cache (TTL only).
const accessCheckSubject = "lfx.access_check.request"

const (
	accessPollInterval   = 250 * time.Millisecond
	accessRequestTimeout = 2 * time.Second
	deliveryPollInterval = 100 * time.Millisecond
	// applyGrace is fga-sync's usual time from a JetStream publish to the OpenFGA write.
	applyGrace = 500 * time.Millisecond
)

var accessTracer = otel.Tracer("fga-access-waiter")

// Requester is the subset of *nats.Conn the access waiter needs.
type Requester interface {
	RequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error)
}

// OutboxDelivery reports whether an object still has FGA markers the relay has not published.
type OutboxDelivery interface {
	HasUndelivered(ctx context.Context, objectType, objectUID string) (bool, error)
}

// AccessWaiter polls fga-sync until a relation is visible or its timeout ends.
type AccessWaiter struct {
	conn         Requester
	timeout      time.Duration
	pollInterval time.Duration
	// outbox gates the first check: a check made before the tuple exists is cached as a
	// denial by OpenFGA (check-query cache) and by fga-sync, outliving the whole wait.
	outbox       OutboxDelivery
	deliveryPoll time.Duration
	applyGrace   time.Duration
}

var _ domain.AccessWaiter = (*AccessWaiter)(nil)

// NewAccessWaiter returns an AccessWaiter that gives up after timeout.
func NewAccessWaiter(conn Requester, timeout time.Duration) *AccessWaiter {
	return &AccessWaiter{conn: conn, timeout: timeout, pollInterval: accessPollInterval, deliveryPoll: deliveryPollInterval, applyGrace: applyGrace}
}

// SetOutboxDelivery makes the waiter hold its first check until the relay has published the object's markers.
func (w *AccessWaiter) SetOutboxDelivery(outbox OutboxDelivery) {
	w.outbox = outbox
}

// WaitForAccess implements domain.AccessWaiter.
func (w *AccessWaiter) WaitForAccess(ctx context.Context, object, relation, username string) error {
	ctx, span := accessTracer.Start(ctx, "fga.WaitForAccess", trace.WithAttributes(
		attribute.String("fga.object", object),
		attribute.String("fga.relation", relation),
	))
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	if err := w.awaitDelivery(ctx, span, object); err != nil {
		err = fmt.Errorf("wait for %s#%s before any check: %w", object, relation, err)
		span.SetAttributes(attribute.Int("fga.attempts", 0))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	check := object + "#" + relation + "@user:" + username
	var lastErr error
	for attempt := 1; ; attempt++ {
		allowed, err := checkAccess(ctx, w.conn, check)
		if err == nil && allowed {
			span.SetAttributes(attribute.Int("fga.attempts", attempt))
			return nil
		}
		if err != nil {
			lastErr = err
		}
		timer := time.NewTimer(w.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			err := fmt.Errorf("wait for %s#%s after %d checks: %w", object, relation, attempt, errors.Join(ctx.Err(), lastErr))
			span.SetAttributes(attribute.Int("fga.attempts", attempt))
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		case <-timer.C:
		}
	}
}

// awaitDelivery returns once the object's outbox markers are published and fga-sync has had
// applyGrace to write them. A failed outbox read does not hold the wait: the checks run anyway.
func (w *AccessWaiter) awaitDelivery(ctx context.Context, span trace.Span, object string) error {
	if w.outbox == nil {
		return nil
	}
	objectType, objectUID, ok := strings.Cut(object, ":")
	if !ok {
		return nil
	}
	started := time.Now()
	for {
		pending, err := w.outbox.HasUndelivered(ctx, objectType, objectUID)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			span.RecordError(fmt.Errorf("read outbox delivery: %w", err))
			return nil
		}
		if !pending {
			span.SetAttributes(attribute.Int64("fga.outbox_delivery_ms", time.Since(started).Milliseconds()))
			return sleep(ctx, w.applyGrace)
		}
		if err := sleep(ctx, w.deliveryPoll); err != nil {
			return fmt.Errorf("outbox not yet published: %w", err)
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// AccessChecker asks fga-sync once whether a relation holds.
type AccessChecker struct {
	conn Requester
}

var _ domain.AccessChecker = (*AccessChecker)(nil)

// NewAccessChecker returns an AccessChecker that asks fga-sync over conn.
func NewAccessChecker(conn Requester) *AccessChecker {
	return &AccessChecker{conn: conn}
}

// HasAccess implements domain.AccessChecker.
func (c *AccessChecker) HasAccess(ctx context.Context, object, relation, username string) (bool, error) {
	ctx, span := accessTracer.Start(ctx, "fga.HasAccess", trace.WithAttributes(
		attribute.String("fga.object", object),
		attribute.String("fga.relation", relation),
	))
	defer span.End()

	allowed, err := checkAccess(ctx, c.conn, object+"#"+relation+"@user:"+username)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return allowed, err
}

// checkAccess asks fga-sync for one relation. The reply is one line per check,
// "{check}\t{true|false}", in no guaranteed order.
func checkAccess(ctx context.Context, conn Requester, check string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, accessRequestTimeout)
	defer cancel()

	reply, err := conn.RequestMsgWithContext(ctx, &nats.Msg{Subject: accessCheckSubject, Data: []byte(check)})
	if err != nil {
		return false, fmt.Errorf("access check: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(reply.Data)), "\n") {
		request, result, found := strings.Cut(line, "\t")
		if found && request == check {
			return strings.TrimSpace(result) == "true", nil
		}
	}
	return false, errors.New("access check: reply has no answer for the requested relation")
}
