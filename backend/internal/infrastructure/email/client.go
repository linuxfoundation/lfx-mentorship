// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package email sends pre-rendered transactional email through lfx-v2-email-service over NATS.
package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// FromDisplayName distinguishes Mentorship mail in the inbox; the address stays the platform default.
const FromDisplayName = "LFX Mentorship"

const defaultTimeout = 10 * time.Second

var tracer = otel.Tracer("email-client")

// ErrRejected marks a send the email service refused; the relay does not retry it.
var ErrRejected = errors.New("email service rejected send")

// Requester is the subset of *nats.Conn the client needs.
type Requester interface {
	RequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error)
}

// Message is one pre-rendered email to a single recipient.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Receipt identifies an accepted send. Acceptance is not delivery.
type Receipt struct {
	EmailID string
}

// Config configures a Client.
type Config struct {
	// Timeout bounds each request when the caller's context has a later or no deadline.
	Timeout time.Duration
}

// Client sends email via the email service's request/reply subject.
type Client struct {
	conn    Requester
	timeout time.Duration
}

// NewClient returns a Client that sends over conn.
func NewClient(conn Requester, cfg Config) *Client {
	c := &Client{conn: conn, timeout: cfg.Timeout}
	if c.timeout <= 0 {
		c.timeout = defaultTimeout
	}
	return c
}

// Send delivers msg to the email service and returns its receipt once accepted.
func (c *Client) Send(ctx context.Context, msg Message) (Receipt, error) {
	data, err := json.Marshal(emailapi.SendEmailRequest{
		To:              msg.To,
		Subject:         msg.Subject,
		HTML:            msg.HTML,
		Text:            msg.Text,
		FromDisplayName: FromDisplayName,
	})
	if err != nil {
		return Receipt{}, fmt.Errorf("marshal send email request: %w", err)
	}

	ctx, span := tracer.Start(ctx, "nats.request "+emailapi.SendEmailSubject,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", emailapi.SendEmailSubject),
			attribute.Int("messaging.message.body.size", len(data)),
		),
	)
	defer span.End()

	receipt, err := c.request(ctx, data)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Receipt{}, err
	}
	span.SetAttributes(attribute.String("email.email_id", receipt.EmailID))
	return receipt, nil
}

func (c *Client) request(ctx context.Context, data []byte) (Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req := nats.NewMsg(emailapi.SendEmailSubject)
	req.Data = data
	// NATS headers are case-sensitive; http.Header canonicalisation would hide traceparent from the consumer.
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		req.Header.Set(k, v)
	}

	reply, err := c.conn.RequestMsgWithContext(ctx, req)
	if err != nil {
		return Receipt{}, fmt.Errorf("email service request: %w", err)
	}

	var errResp emailapi.SendEmailErrorResponse
	if err := json.Unmarshal(reply.Data, &errResp); err == nil && errResp.Error != "" {
		return Receipt{}, fmt.Errorf("%w: %s", ErrRejected, errResp.Error)
	}
	// Decode email_id as a pointer: an allowlist-blocked recipient gets "" (counted as sent), but a
	// reply without the field is not a send reply at all.
	var resp struct {
		EmailID *string `json:"email_id"`
	}
	if err := json.Unmarshal(reply.Data, &resp); err != nil {
		return Receipt{}, fmt.Errorf("decode email service reply: %w", err)
	}
	if resp.EmailID == nil {
		return Receipt{}, errors.New("decode email service reply: no email_id")
	}
	return Receipt{EmailID: *resp.EmailID}, nil
}
