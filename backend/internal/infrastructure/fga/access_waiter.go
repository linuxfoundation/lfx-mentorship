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
// OpenFGA (through a cache invalidated on every tuple write), the store
// Heimdall's openfga_check authorizer reads.
const accessCheckSubject = "lfx.access_check.request"

const (
	accessPollInterval   = 250 * time.Millisecond
	accessRequestTimeout = 2 * time.Second
)

var accessTracer = otel.Tracer("fga-access-waiter")

// Requester is the subset of *nats.Conn the access waiter needs.
type Requester interface {
	RequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error)
}

// AccessWaiter polls fga-sync until a relation is visible or its timeout ends.
type AccessWaiter struct {
	conn         Requester
	timeout      time.Duration
	pollInterval time.Duration
}

var _ domain.AccessWaiter = (*AccessWaiter)(nil)

// NewAccessWaiter returns an AccessWaiter that gives up after timeout.
func NewAccessWaiter(conn Requester, timeout time.Duration) *AccessWaiter {
	return &AccessWaiter{conn: conn, timeout: timeout, pollInterval: accessPollInterval}
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

	check := object + "#" + relation + "@user:" + username
	var lastErr error
	for attempt := 1; ; attempt++ {
		allowed, err := w.check(ctx, check)
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

// check asks fga-sync for one relation. The reply is one line per check,
// "{check}\t{true|false}", in no guaranteed order.
func (w *AccessWaiter) check(ctx context.Context, check string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, accessRequestTimeout)
	defer cancel()

	reply, err := w.conn.RequestMsgWithContext(ctx, &nats.Msg{Subject: accessCheckSubject, Data: []byte(check)})
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
