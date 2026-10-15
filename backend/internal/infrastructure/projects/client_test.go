// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package projects

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const projectUID = "7cad5a8d-19d0-41a4-81a6-043453daf9ee"

// requesterStub answers each subject with its reply, or fails with err.
type requesterStub struct {
	replies  map[string]string
	err      error
	got      []*nats.Msg
	deadline time.Time
}

func (s *requesterStub) RequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error) {
	s.got = append(s.got, msg)
	s.deadline, _ = ctx.Deadline()
	if s.err != nil {
		return nil, s.err
	}
	return &nats.Msg{Data: []byte(s.replies[msg.Subject])}, nil
}

func projectReplies(slug, name, logo string) map[string]string {
	return map[string]string{
		constants.ProjectGetSlugSubject: slug,
		constants.ProjectGetNameSubject: name,
		constants.ProjectGetLogoSubject: logo,
	}
}

func TestGetProject_Success(t *testing.T) {
	stub := &requesterStub{replies: projectReplies("cncf", " Cloud Native Computing Foundation ", "https://example.org/cncf.png")}

	project, err := NewClient(stub).GetProject(context.Background(), projectUID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if project.Slug != "cncf" || project.Name != "Cloud Native Computing Foundation" || project.LogoURL == nil || *project.LogoURL != "https://example.org/cncf.png" {
		t.Fatalf("project = %+v", project)
	}
	if len(stub.got) != 3 {
		t.Fatalf("sent %d requests, want 3", len(stub.got))
	}
	for _, msg := range stub.got {
		if string(msg.Data) != projectUID {
			t.Errorf("%s data = %q, want the project UID", msg.Subject, msg.Data)
		}
	}
}

func TestGetProject_NoLogo(t *testing.T) {
	project, err := NewClient(&requesterStub{replies: projectReplies("cncf", "CNCF", "")}).GetProject(context.Background(), projectUID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if project.LogoURL != nil {
		t.Fatalf("logo = %q, want nil", *project.LogoURL)
	}
}

func TestGetProject_AppliesTimeout(t *testing.T) {
	stub := &requesterStub{replies: projectReplies("cncf", "CNCF", "")}

	before := time.Now()
	if _, err := NewClient(stub).GetProject(context.Background(), projectUID); err != nil {
		t.Fatal(err)
	}
	if stub.deadline.IsZero() || stub.deadline.After(before.Add(defaultTimeout+time.Second)) {
		t.Fatalf("deadline = %v, want ~%v from %v", stub.deadline, defaultTimeout, before)
	}
}

func TestGetProject_PropagatesTraceContext(t *testing.T) {
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	stub := &requesterStub{replies: projectReplies("cncf", "CNCF", "")}

	if _, err := NewClient(stub).GetProject(ctx, projectUID); err != nil {
		t.Fatal(err)
	}
	if stub.got[0].Header.Get("traceparent") == "" {
		t.Fatalf("lowercase traceparent header missing: %v", stub.got[0].Header)
	}
}

func TestGetProject_NotFound(t *testing.T) {
	stub := &requesterStub{replies: projectReplies(`{"error":"not_found","message":"project not found"}`, "", "")}

	_, err := NewClient(stub).GetProject(context.Background(), projectUID)
	if !errors.Is(err, domain.ErrProjectNotFound) || errors.Is(err, domain.ErrUpstreamUnavailable) {
		t.Fatalf("err = %v, want ErrProjectNotFound", err)
	}
	if len(stub.got) != 1 {
		t.Fatalf("sent %d requests, want 1", len(stub.got))
	}
}

func TestGetProject_Unavailable(t *testing.T) {
	for name, stub := range map[string]*requesterStub{
		"transport error": {err: nats.ErrNoResponders},
		"internal error":  {replies: projectReplies(`{"error":"internal","message":"internal server error"}`, "", "")},
		"empty slug":      {replies: projectReplies("", "CNCF", "")},
		"empty name":      {replies: projectReplies("cncf", " ", "")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewClient(stub).GetProject(context.Background(), projectUID)
			if !errors.Is(err, domain.ErrUpstreamUnavailable) || errors.Is(err, domain.ErrProjectNotFound) {
				t.Fatalf("err = %v, want ErrUpstreamUnavailable", err)
			}
		})
	}
}
