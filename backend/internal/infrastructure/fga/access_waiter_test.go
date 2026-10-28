// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	waitObject = "mentorship_program:p1"
	waitCheck  = waitObject + "#writer@user:alice"
	waitTuple  = waitObject + "#writer@user:alice"
	// otherTuple is a direct tuple alice holds on another program; it must not satisfy the wait.
	otherTuple = "mentorship_program:p2#writer@user:alice"
)

type reply func(*nats.Msg) (*nats.Msg, error)

// requesterStub answers each subject with its next reply in turn, repeating the last,
// and records every request in order.
type requesterStub struct {
	replies  map[string][]reply
	requests []*nats.Msg
}

func (s *requesterStub) RequestMsgWithContext(_ context.Context, msg *nats.Msg) (*nats.Msg, error) {
	s.requests = append(s.requests, msg)
	n := 0
	for _, r := range s.requests {
		if r.Subject == msg.Subject {
			n++
		}
	}
	replies := s.replies[msg.Subject]
	if len(replies) == 0 {
		return nil, errors.New("unexpected subject " + msg.Subject)
	}
	return replies[min(n, len(replies))-1](msg)
}

func (s *requesterStub) subjects() []string {
	out := make([]string, 0, len(s.requests))
	for _, r := range s.requests {
		out = append(out, r.Subject)
	}
	return out
}

func answer(body string) reply {
	return func(*nats.Msg) (*nats.Msg, error) { return &nats.Msg{Data: []byte(body)}, nil }
}

func fail(err error) reply {
	return func(*nats.Msg) (*nats.Msg, error) { return nil, err }
}

// tuples answers read_tuples with the given direct tuples.
func tuples(results ...string) reply {
	body, _ := json.Marshal(readTuplesResponse{Results: results})
	return answer(string(body))
}

func newTestWaiter(conn Requester, timeout time.Duration) *AccessWaiter {
	w := NewAccessWaiter(conn, timeout)
	w.pollInterval = time.Millisecond
	return w
}

// checkerStub answers only the check subject, for the one-shot AccessChecker.
func checkerStub(r reply) *requesterStub {
	return &requesterStub{replies: map[string][]reply{accessCheckSubject: {r}}}
}

func TestAccessWaiter_ReadsTupleBeforeAnyCheck(t *testing.T) {
	conn := &requesterStub{replies: map[string][]reply{
		readTuplesSubject: {
			tuples(),
			answer(`{"error":"failed to read tuples"}`),
			tuples(otherTuple),
			tuples(otherTuple, waitTuple),
		},
		accessCheckSubject: {
			answer(waitCheck + "\tfalse"),
			answer("other#viewer@user:bob\ttrue\n" + waitCheck + "\ttrue\n"),
		},
	}}

	if err := newTestWaiter(conn, time.Second).WaitForAccess(context.Background(), waitObject, "writer", "alice"); err != nil {
		t.Fatalf("WaitForAccess: %v", err)
	}
	want := []string{readTuplesSubject, readTuplesSubject, readTuplesSubject, readTuplesSubject, accessCheckSubject, accessCheckSubject}
	if got := conn.subjects(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("requests = %v; want every read before the first check: %v", got, want)
	}
	var read readTuplesRequest
	if err := json.Unmarshal(conn.requests[0].Data, &read); err != nil || read.User != "user:alice" || read.ObjectType != "mentorship_program" {
		t.Fatalf("read request = %+v (%v); want user:alice on mentorship_program", read, err)
	}
	if string(conn.requests[4].Data) != waitCheck {
		t.Fatalf("check = %q; want %q", conn.requests[4].Data, waitCheck)
	}
}

func TestAccessWaiter_NeverChecksWithoutTheTuple(t *testing.T) {
	for label, r := range map[string]reply{
		"no tuples":         tuples(),
		"other object only": tuples(otherTuple),
		"read error":        answer(`{"error":"failed to read tuples"}`),
		"malformed reply":   answer("not json"),
	} {
		t.Run(label, func(t *testing.T) {
			conn := &requesterStub{replies: map[string][]reply{
				readTuplesSubject:  {r},
				accessCheckSubject: {answer(waitCheck + "\ttrue")},
			}}
			err := newTestWaiter(conn, 20*time.Millisecond).WaitForAccess(context.Background(), waitObject, "writer", "alice")
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "no tuple for alice") {
				t.Fatalf("err = %v; want a deadline with no tuple", err)
			}
			for _, subject := range conn.subjects() {
				if subject == accessCheckSubject {
					t.Fatal("a check ran without the tuple; that denial would be cached past the wait")
				}
			}
		})
	}
}

func TestAccessWaiter_TimesOutOnChecks(t *testing.T) {
	for label, r := range map[string]reply{
		"never allowed": answer(waitCheck + "\tfalse"),
		"no answer":     answer("other#viewer@user:bob\ttrue"),
	} {
		t.Run(label, func(t *testing.T) {
			conn := &requesterStub{replies: map[string][]reply{
				readTuplesSubject:  {tuples(waitTuple)},
				accessCheckSubject: {r},
			}}
			err := newTestWaiter(conn, 20*time.Millisecond).WaitForAccess(context.Background(), waitObject, "writer", "alice")
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), waitObject+"#writer after") {
				t.Fatalf("err = %v; want a deadline naming the object and relation after checks", err)
			}
		})
	}
}

func TestAccessWaiter_StopsWhenCallerCancels(t *testing.T) {
	conn := &requesterStub{replies: map[string][]reply{readTuplesSubject: {tuples()}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := newTestWaiter(conn, time.Minute).WaitForAccess(ctx, waitObject, "writer", "alice"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want Canceled", err)
	}
}

// An unanswered request ends the wait at once: fga-sync keeps serving it for up to
// 10s after our deadline, so retrying every 250ms would pile work onto handler slots
// shared with the gateway's checks, and a missing responder will not appear in time.
func TestAccessWaiter_EndsWhenFGASyncDoesNotAnswer(t *testing.T) {
	for label, tc := range map[string]struct {
		reads, checks []reply
		cause         error
		wantSubjects  []string
		wantErr       string
	}{
		"no read responder": {
			reads:        []reply{fail(nats.ErrNoResponders)},
			cause:        nats.ErrNoResponders,
			wantSubjects: []string{readTuplesSubject},
			wantErr:      "did not answer",
		},
		"read deadline": {
			reads:        []reply{fail(context.DeadlineExceeded)},
			cause:        context.DeadlineExceeded,
			wantSubjects: []string{readTuplesSubject},
			wantErr:      "did not answer",
		},
		"no check responder": {
			reads:        []reply{tuples(waitTuple)},
			checks:       []reply{fail(nats.ErrNoResponders)},
			cause:        nats.ErrNoResponders,
			wantSubjects: []string{readTuplesSubject, accessCheckSubject},
			wantErr:      "did not answer check 1",
		},
		"check deadline": {
			reads:        []reply{tuples(waitTuple)},
			checks:       []reply{fail(context.DeadlineExceeded)},
			cause:        context.DeadlineExceeded,
			wantSubjects: []string{readTuplesSubject, accessCheckSubject},
			wantErr:      "did not answer check 1",
		},
	} {
		t.Run(label, func(t *testing.T) {
			conn := &requesterStub{replies: map[string][]reply{readTuplesSubject: tc.reads, accessCheckSubject: tc.checks}}
			started := time.Now()
			err := newTestWaiter(conn, time.Minute).WaitForAccess(context.Background(), waitObject, "writer", "alice")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !errors.Is(err, tc.cause) {
				t.Fatalf("err = %v; want %q wrapping %v", err, tc.wantErr, tc.cause)
			}
			if time.Since(started) > time.Second {
				t.Fatalf("wait took %s; want it to end at once", time.Since(started))
			}
			if got := conn.subjects(); strings.Join(got, ",") != strings.Join(tc.wantSubjects, ",") {
				t.Fatalf("requests = %v; want %v and nothing after", got, tc.wantSubjects)
			}
		})
	}
}

func TestAccessWaiter_RejectsObjectWithoutType(t *testing.T) {
	conn := &requesterStub{}
	err := newTestWaiter(conn, time.Second).WaitForAccess(context.Background(), "p1", "writer", "alice")
	if err == nil || !strings.Contains(err.Error(), "no type prefix") || len(conn.requests) != 0 {
		t.Fatalf("err = %v, requests = %d; want an immediate error and no requests", err, len(conn.requests))
	}
}

func TestAccessChecker_AsksOnce(t *testing.T) {
	for label, tc := range map[string]struct {
		reply   reply
		allowed bool
		wantErr bool
	}{
		"allowed":            {reply: answer("other#viewer@user:bob\tfalse\n" + waitCheck + "\ttrue\n"), allowed: true},
		"denied":             {reply: answer(waitCheck + "\tfalse")},
		"no answer":          {reply: answer("other#viewer@user:bob\ttrue"), wantErr: true},
		"fga-sync unreached": {reply: fail(nats.ErrNoResponders), wantErr: true},
	} {
		t.Run(label, func(t *testing.T) {
			conn := checkerStub(tc.reply)
			allowed, err := NewAccessChecker(conn).HasAccess(context.Background(), waitObject, "writer", "alice")
			if allowed != tc.allowed || (err != nil) != tc.wantErr {
				t.Fatalf("allowed = %v, err = %v; want %v, error %v", allowed, err, tc.allowed, tc.wantErr)
			}
			if len(conn.requests) != 1 || string(conn.requests[0].Data) != waitCheck {
				t.Fatalf("requests = %d; want one %q", len(conn.requests), waitCheck)
			}
		})
	}
}
