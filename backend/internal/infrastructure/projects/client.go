// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package projects resolves LF project metadata from lfx-v2-project-service over NATS request/reply.
package projects

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const defaultTimeout = 10 * time.Second

var tracer = otel.Tracer("projects-client")

// Requester is the subset of *nats.Conn the client needs.
type Requester interface {
	RequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error)
}

// Client implements domain.ProjectLookup with Project Service's attribute lookups.
type Client struct {
	conn    Requester
	timeout time.Duration
}

var _ domain.ProjectLookup = (*Client)(nil)

// NewClient returns a Client that sends over conn.
func NewClient(conn Requester) *Client {
	return &Client{conn: conn, timeout: defaultTimeout}
}

// GetProject returns the slug, name, and logo of the project with the given UID.
func (c *Client) GetProject(ctx context.Context, uid string) (*models.ProjectMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	slug, err := c.request(ctx, constants.ProjectGetSlugSubject, uid)
	if err != nil {
		return nil, err
	}
	name, err := c.request(ctx, constants.ProjectGetNameSubject, uid)
	if err != nil {
		return nil, err
	}
	logo, err := c.request(ctx, constants.ProjectGetLogoSubject, uid)
	if err != nil {
		return nil, err
	}
	// Only get_logo answers with an empty body on purpose (no logo set); on the
	// other subjects it means the request was not dispatched.
	if slug == "" || name == "" {
		return nil, fmt.Errorf("project service returned an empty slug or name for project %s: %w", uid, domain.ErrUpstreamUnavailable)
	}
	project := &models.ProjectMetadata{Slug: slug, Name: name}
	if logo != "" {
		project.LogoURL = &logo
	}
	return project, nil
}

func (c *Client) request(ctx context.Context, subject, uid string) (string, error) {
	ctx, span := tracer.Start(ctx, "nats.request "+subject,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.String("project.uid", uid),
		),
	)
	defer span.End()

	value, err := c.send(ctx, subject, uid)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	return value, nil
}

func (c *Client) send(ctx context.Context, subject, uid string) (string, error) {
	req := nats.NewMsg(subject)
	req.Data = []byte(uid)
	// NATS headers are case-sensitive; http.Header canonicalisation would hide traceparent from the consumer.
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		req.Header.Set(k, v)
	}

	reply, err := c.conn.RequestMsgWithContext(ctx, req)
	if err != nil {
		return "", fmt.Errorf("project service %s: %w: %w", subject, err, domain.ErrUpstreamUnavailable)
	}
	if rpcErr, ok := events.ParseRPCError(reply.Data); ok {
		if errors.Is(rpcErr, events.ErrRPCNotFound) {
			return "", fmt.Errorf("project %s: %w", uid, domain.ErrProjectNotFound)
		}
		return "", fmt.Errorf("project service %s: %w: %w", subject, rpcErr, domain.ErrUpstreamUnavailable)
	}
	return strings.TrimSpace(string(reply.Data)), nil
}
