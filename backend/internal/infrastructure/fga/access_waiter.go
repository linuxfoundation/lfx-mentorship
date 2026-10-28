// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"encoding/json"
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

// readTuplesSubject is fga-sync's request/reply read of a user's direct tuples on an
// object type, straight from OpenFGA's Read API: no cache sits in front of it, and
// asking leaves nothing cached behind.
const readTuplesSubject = "lfx.access_check.read_tuples"

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
//
// It asks for the relation only once the user's direct tuple on the object exists.
// A check made before then is answered false and cached, by OpenFGA's check-query
// cache for its TTL and by fga-sync's cache after that, past the end of the wait.
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

	fail := func(err error) error {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	reads, err := w.awaitTuple(ctx, object, username)
	span.SetAttributes(attribute.Int("fga.tuple_reads", reads))
	if err != nil {
		return fail(fmt.Errorf("wait for %s#%s: no tuple for %s after %d reads: %w", object, relation, username, reads, err))
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
		if err := sleep(ctx, w.pollInterval); err != nil {
			span.SetAttributes(attribute.Int("fga.attempts", attempt))
			return fail(fmt.Errorf("wait for %s#%s after %d checks: %w", object, relation, attempt, errors.Join(err, lastErr)))
		}
	}
}

// awaitTuple polls read_tuples until user:{username} holds a direct tuple on object,
// returning the number of reads made. fga-sync writes the direct tuple and the relations
// derived from it in one OpenFGA write, so its presence means the grant is applied.
func (w *AccessWaiter) awaitTuple(ctx context.Context, object, username string) (int, error) {
	objectType, _, ok := strings.Cut(object, ":")
	if !ok {
		return 0, fmt.Errorf("object %q has no type prefix", object)
	}
	request, err := json.Marshal(readTuplesRequest{User: "user:" + username, ObjectType: objectType})
	if err != nil {
		return 0, err
	}
	var lastErr error
	for reads := 1; ; reads++ {
		found, err := w.readTuple(ctx, request, object+"#")
		if err == nil && found {
			return reads, nil
		}
		if err != nil {
			lastErr = err
		}
		if err := sleep(ctx, w.pollInterval); err != nil {
			return reads, errors.Join(err, lastErr)
		}
	}
}

type readTuplesRequest struct {
	User       string `json:"user"`
	ObjectType string `json:"object_type"`
}

type readTuplesResponse struct {
	Results []string `json:"results"`
	Error   string   `json:"error,omitempty"`
}

// readTuple reports whether any of the user's direct tuples is on the object, given
// as the "{object}#" prefix of fga-sync's "{object}#{relation}@{user}" result lines.
func (w *AccessWaiter) readTuple(ctx context.Context, request []byte, objectPrefix string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, accessRequestTimeout)
	defer cancel()

	reply, err := w.conn.RequestMsgWithContext(ctx, &nats.Msg{Subject: readTuplesSubject, Data: request})
	if err != nil {
		return false, fmt.Errorf("read tuples: %w", err)
	}
	var response readTuplesResponse
	if err := json.Unmarshal(reply.Data, &response); err != nil {
		return false, fmt.Errorf("read tuples: %w", err)
	}
	if response.Error != "" {
		return false, fmt.Errorf("read tuples: %s", response.Error)
	}
	for _, result := range response.Results {
		if strings.HasPrefix(result, objectPrefix) {
			return true, nil
		}
	}
	return false, nil
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
