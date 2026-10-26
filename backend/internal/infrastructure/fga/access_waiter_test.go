// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

const waitCheck = "mentorship_program:p1#writer@user:alice"

// requesterStub answers each request with the next reply in turn, repeating the last.
type requesterStub struct {
	replies  []func(*nats.Msg) (*nats.Msg, error)
	requests []*nats.Msg
}

func (s *requesterStub) RequestMsgWithContext(_ context.Context, msg *nats.Msg) (*nats.Msg, error) {
	s.requests = append(s.requests, msg)
	reply := s.replies[min(len(s.requests), len(s.replies))-1]
	return reply(msg)
}

func answer(body string) func(*nats.Msg) (*nats.Msg, error) {
	return func(*nats.Msg) (*nats.Msg, error) { return &nats.Msg{Data: []byte(body)}, nil }
}

func fail(err error) func(*nats.Msg) (*nats.Msg, error) {
	return func(*nats.Msg) (*nats.Msg, error) { return nil, err }
}

func newTestWaiter(conn Requester, timeout time.Duration) *AccessWaiter {
	w := NewAccessWaiter(conn, timeout)
	w.pollInterval = time.Millisecond
	return w
}

func TestAccessWaiter_PollsUntilAllowed(t *testing.T) {
	conn := &requesterStub{replies: []func(*nats.Msg) (*nats.Msg, error){
		answer(waitCheck + "\tfalse"),
		fail(nats.ErrNoResponders),
		answer("other#viewer@user:bob\ttrue\n" + waitCheck + "\ttrue\n"),
	}}

	if err := newTestWaiter(conn, time.Second).WaitForAccess(context.Background(), "mentorship_program:p1", "writer", "alice"); err != nil {
		t.Fatalf("WaitForAccess: %v", err)
	}
	if len(conn.requests) != 3 {
		t.Fatalf("requests = %d; want 3", len(conn.requests))
	}
	for _, request := range conn.requests {
		if request.Subject != "lfx.access_check.request" || string(request.Data) != waitCheck {
			t.Errorf("request %s %q; want lfx.access_check.request %q", request.Subject, request.Data, waitCheck)
		}
	}
}

func TestAccessWaiter_TimesOut(t *testing.T) {
	for label, reply := range map[string]func(*nats.Msg) (*nats.Msg, error){
		"never allowed":      answer(waitCheck + "\tfalse"),
		"no answer":          answer("other#viewer@user:bob\ttrue"),
		"fga-sync unreached": fail(nats.ErrNoResponders),
	} {
		t.Run(label, func(t *testing.T) {
			conn := &requesterStub{replies: []func(*nats.Msg) (*nats.Msg, error){reply}}
			err := newTestWaiter(conn, 20*time.Millisecond).WaitForAccess(context.Background(), "mentorship_program:p1", "writer", "alice")
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err = %v; want DeadlineExceeded", err)
			}
			if !strings.Contains(err.Error(), "mentorship_program:p1#writer") {
				t.Errorf("err = %q; want it to name the object and relation", err)
			}
		})
	}
}

func TestAccessWaiter_StopsWhenCallerCancels(t *testing.T) {
	conn := &requesterStub{replies: []func(*nats.Msg) (*nats.Msg, error){answer(waitCheck + "\tfalse")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := newTestWaiter(conn, time.Minute).WaitForAccess(ctx, "mentorship_program:p1", "writer", "alice"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want Canceled", err)
	}
}
