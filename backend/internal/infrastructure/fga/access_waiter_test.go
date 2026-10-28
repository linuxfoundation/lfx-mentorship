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
	w.deliveryPoll = time.Millisecond
	w.applyGrace = time.Millisecond
	return w
}

// outboxStub reports undelivered markers for the first pendingReads reads.
type outboxStub struct {
	pendingReads int
	err          error
	reads        []string
	conn         *requesterStub
	checksBefore int
}

func (o *outboxStub) HasUndelivered(_ context.Context, objectType, objectUID string) (bool, error) {
	o.reads = append(o.reads, objectType+":"+objectUID)
	if o.conn != nil && len(o.conn.requests) > 0 {
		o.checksBefore++
	}
	if o.err != nil {
		return false, o.err
	}
	return len(o.reads) <= o.pendingReads, nil
}

func TestAccessWaiter_FirstCheckWaitsForOutboxDelivery(t *testing.T) {
	conn := &requesterStub{replies: []func(*nats.Msg) (*nats.Msg, error){answer(waitCheck + "\ttrue")}}
	outbox := &outboxStub{pendingReads: 3, conn: conn}
	w := newTestWaiter(conn, time.Second)
	w.SetOutboxDelivery(outbox)

	if err := w.WaitForAccess(context.Background(), "mentorship_program:p1", "writer", "alice"); err != nil {
		t.Fatalf("WaitForAccess: %v", err)
	}
	if len(outbox.reads) != 4 || outbox.reads[0] != "mentorship_program:p1" {
		t.Fatalf("outbox reads = %v; want 4 reads of mentorship_program:p1", outbox.reads)
	}
	if outbox.checksBefore != 0 || len(conn.requests) != 1 {
		t.Fatalf("%d checks overlapped undelivered markers, %d checks in all; want none before delivery and one after", outbox.checksBefore, len(conn.requests))
	}
}

func TestAccessWaiter_NeverChecksWhileUndelivered(t *testing.T) {
	conn := &requesterStub{replies: []func(*nats.Msg) (*nats.Msg, error){answer(waitCheck + "\ttrue")}}
	w := newTestWaiter(conn, 20*time.Millisecond)
	w.SetOutboxDelivery(&outboxStub{pendingReads: 1 << 30})

	err := w.WaitForAccess(context.Background(), "mentorship_program:p1", "writer", "alice")
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "before any check") {
		t.Fatalf("err = %v; want a deadline before any check", err)
	}
	if len(conn.requests) != 0 {
		t.Fatalf("requests = %d; a check before delivery would cache a denial", len(conn.requests))
	}
}

func TestAccessWaiter_OutboxReadFailureStillChecks(t *testing.T) {
	conn := &requesterStub{replies: []func(*nats.Msg) (*nats.Msg, error){answer(waitCheck + "\ttrue")}}
	w := newTestWaiter(conn, time.Second)
	w.SetOutboxDelivery(&outboxStub{err: errors.New("db down")})

	if err := w.WaitForAccess(context.Background(), "mentorship_program:p1", "writer", "alice"); err != nil {
		t.Fatalf("WaitForAccess: %v", err)
	}
	if len(conn.requests) != 1 {
		t.Fatalf("requests = %d; want the check to run despite the outbox error", len(conn.requests))
	}
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

func TestAccessChecker_AsksOnce(t *testing.T) {
	for label, tc := range map[string]struct {
		reply   func(*nats.Msg) (*nats.Msg, error)
		allowed bool
		wantErr bool
	}{
		"allowed":            {reply: answer("other#viewer@user:bob\tfalse\n" + waitCheck + "\ttrue\n"), allowed: true},
		"denied":             {reply: answer(waitCheck + "\tfalse")},
		"no answer":          {reply: answer("other#viewer@user:bob\ttrue"), wantErr: true},
		"fga-sync unreached": {reply: fail(nats.ErrNoResponders), wantErr: true},
	} {
		t.Run(label, func(t *testing.T) {
			conn := &requesterStub{replies: []func(*nats.Msg) (*nats.Msg, error){tc.reply}}
			allowed, err := NewAccessChecker(conn).HasAccess(context.Background(), "mentorship_program:p1", "writer", "alice")
			if allowed != tc.allowed || (err != nil) != tc.wantErr {
				t.Fatalf("allowed = %v, err = %v; want %v, error %v", allowed, err, tc.allowed, tc.wantErr)
			}
			if len(conn.requests) != 1 || string(conn.requests[0].Data) != waitCheck {
				t.Fatalf("requests = %d; want one %q", len(conn.requests), waitCheck)
			}
		})
	}
}
