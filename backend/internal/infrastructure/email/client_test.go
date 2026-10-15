// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type requesterStub struct {
	reply    []byte
	err      error
	got      *nats.Msg
	deadline time.Time
}

func (s *requesterStub) RequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error) {
	s.got = msg
	s.deadline, _ = ctx.Deadline()
	if s.err != nil {
		return nil, s.err
	}
	return &nats.Msg{Data: s.reply}, nil
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSend_Success(t *testing.T) {
	stub := &requesterStub{reply: mustJSON(t, emailapi.SendEmailResponse{EmailID: "e-1", GroupID: "g-1"})}
	c := NewClient(stub, Config{})

	receipt, err := c.Send(context.Background(), Message{
		To: "mentor@linuxfoundation.org", Subject: "Invite", HTML: "<p>Hi</p>", Text: "Hi",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if receipt != (Receipt{EmailID: "e-1"}) {
		t.Fatalf("receipt = %+v", receipt)
	}
	if stub.got.Subject != emailapi.SendEmailSubject {
		t.Fatalf("subject = %q", stub.got.Subject)
	}
	var sent emailapi.SendEmailRequest
	if err := json.Unmarshal(stub.got.Data, &sent); err != nil {
		t.Fatal(err)
	}
	want := emailapi.SendEmailRequest{
		To: "mentor@linuxfoundation.org", Subject: "Invite", HTML: "<p>Hi</p>", Text: "Hi",
		FromDisplayName: FromDisplayName,
	}
	if sent != want {
		t.Fatalf("request = %+v, want %+v", sent, want)
	}
}

func TestSend_AppliesTimeout(t *testing.T) {
	stub := &requesterStub{reply: mustJSON(t, emailapi.SendEmailResponse{EmailID: "e-1"})}
	c := NewClient(stub, Config{Timeout: time.Second})

	before := time.Now()
	if _, err := c.Send(context.Background(), Message{}); err != nil {
		t.Fatal(err)
	}
	if stub.deadline.IsZero() || stub.deadline.After(before.Add(2*time.Second)) {
		t.Fatalf("deadline = %v, want ~1s from %v", stub.deadline, before)
	}
}

func TestSend_PropagatesTraceContext(t *testing.T) {
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	stub := &requesterStub{reply: mustJSON(t, emailapi.SendEmailResponse{EmailID: "e-1"})}

	if _, err := NewClient(stub, Config{}).Send(ctx, Message{}); err != nil {
		t.Fatal(err)
	}
	if stub.got.Header.Get("traceparent") == "" {
		t.Fatalf("lowercase traceparent header missing: %v", stub.got.Header)
	}
}

func TestSend_Rejected(t *testing.T) {
	stub := &requesterStub{reply: mustJSON(t, emailapi.SendEmailErrorResponse{Error: "to, subject, html, and text are required"})}

	_, err := NewClient(stub, Config{}).Send(context.Background(), Message{})
	if !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
}

func TestSend_TransportError(t *testing.T) {
	stub := &requesterStub{err: nats.ErrNoResponders}

	_, err := NewClient(stub, Config{}).Send(context.Background(), Message{})
	if !errors.Is(err, nats.ErrNoResponders) || errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want wrapped ErrNoResponders", err)
	}
}

func TestSend_MalformedReply(t *testing.T) {
	for name, reply := range map[string]string{
		"not json":         "nope",
		"empty object":     "{}",
		"unrelated fields": `{"status":"ok"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewClient(&requesterStub{reply: []byte(reply)}, Config{}).Send(context.Background(), Message{})
			if err == nil || errors.Is(err, ErrRejected) {
				t.Fatalf("err = %v, want malformed-reply error", err)
			}
		})
	}
}

func TestSend_AllowlistBlockedRecipientCountsAsSent(t *testing.T) {
	receipt, err := NewClient(&requesterStub{reply: []byte(`{"email_id":"","group_id":""}`)}, Config{}).Send(context.Background(), Message{})
	if err != nil || receipt != (Receipt{}) {
		t.Fatalf("receipt, err = %+v, %v; want empty receipt, nil", receipt, err)
	}
}
