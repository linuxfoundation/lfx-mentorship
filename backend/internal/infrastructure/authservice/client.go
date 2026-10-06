// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package authservice looks up LF accounts through lfx-v2-auth-service over NATS.
package authservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	subjectEmailToUsername  = "lfx.auth-service.email_to_username"
	subjectUserMetadataRead = "lfx.auth-service.user_metadata.read"
	subjectUserEmailsRead   = "lfx.auth-service.user_emails.read"

	// errUserNotFound is the error auth-service replies with when no account matches.
	errUserNotFound = "user not found"

	defaultTimeout = 5 * time.Second
)

var tracer = otel.Tracer("auth-service-client")

// Requester is the subset of *nats.Conn the client needs.
type Requester interface {
	RequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error)
}

// Client implements domain.AccountDirectory.
type Client struct {
	conn    Requester
	timeout time.Duration
}

var _ domain.AccountDirectory = (*Client)(nil)

// NewClient returns a Client that requests over conn; a non-positive timeout uses the default.
func NewClient(conn Requester, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{conn: conn, timeout: timeout}
}

// statusReply is the envelope of every JSON reply; a plain-text success reply does not decode into it.
type statusReply struct {
	Success *bool  `json:"success"`
	Error   string `json:"error"`
}

func (r statusReply) err(subject string) error {
	if (r.Success == nil || *r.Success) && r.Error == "" {
		return nil
	}
	if strings.TrimSpace(r.Error) == errUserNotFound {
		return domain.ErrAccountNotFound
	}
	return fmt.Errorf("%w: %s: %s", domain.ErrUpstreamUnavailable, subject, r.Error)
}

// UsernameByEmail returns the LF username whose primary or linked email is email.
func (c *Client) UsernameByEmail(ctx context.Context, email string) (string, error) {
	data, err := c.request(ctx, subjectEmailToUsername, []byte(email))
	if err != nil {
		return "", err
	}
	var status statusReply
	if json.Unmarshal(data, &status) == nil {
		if err := status.err(subjectEmailToUsername); err != nil {
			return "", err
		}
	}
	username := strings.TrimSpace(string(data))
	if username == "" || strings.HasPrefix(username, "{") {
		return "", fmt.Errorf("%w: %s: unexpected reply", domain.ErrUpstreamUnavailable, subjectEmailToUsername)
	}
	return username, nil
}

// Account returns the profile of the LF account with the given username.
func (c *Client) Account(ctx context.Context, username string) (*models.LFAccount, error) {
	data, err := c.request(ctx, subjectUserMetadataRead, []byte(username))
	if err != nil {
		return nil, err
	}
	var reply struct {
		statusReply
		Data *struct {
			Name       string `json:"name"`
			GivenName  string `json:"given_name"`
			FamilyName string `json:"family_name"`
			Picture    string `json:"picture"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return nil, fmt.Errorf("%w: decode %s reply: %v", domain.ErrUpstreamUnavailable, subjectUserMetadataRead, err)
	}
	if err := reply.err(subjectUserMetadataRead); err != nil {
		return nil, err
	}
	// auth-service answers data: null for an account with no profile metadata.
	if reply.Data == nil {
		return &models.LFAccount{Username: username}, nil
	}
	return &models.LFAccount{
		Username:   username,
		Name:       optional(reply.Data.Name),
		GivenName:  optional(reply.Data.GivenName),
		FamilyName: optional(reply.Data.FamilyName),
		AvatarURL:  optional(reply.Data.Picture),
	}, nil
}

// PrimaryEmail returns the primary email of the LF account with the given username.
func (c *Client) PrimaryEmail(ctx context.Context, username string) (string, error) {
	req, err := json.Marshal(map[string]map[string]string{"user": {"auth_token": username}})
	if err != nil {
		return "", fmt.Errorf("marshal %s request: %w", subjectUserEmailsRead, err)
	}
	data, err := c.request(ctx, subjectUserEmailsRead, req)
	if err != nil {
		return "", err
	}
	var reply struct {
		statusReply
		Data *struct {
			PrimaryEmail string `json:"primary_email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return "", fmt.Errorf("%w: decode %s reply: %v", domain.ErrUpstreamUnavailable, subjectUserEmailsRead, err)
	}
	if err := reply.err(subjectUserEmailsRead); err != nil {
		return "", err
	}
	if reply.Data == nil || strings.TrimSpace(reply.Data.PrimaryEmail) == "" {
		return "", fmt.Errorf("%w: %s: reply has no primary email", domain.ErrUpstreamUnavailable, subjectUserEmailsRead)
	}
	return strings.TrimSpace(reply.Data.PrimaryEmail), nil
}

func (c *Client) request(ctx context.Context, subject string, data []byte) ([]byte, error) {
	ctx, span := tracer.Start(ctx, "nats.request "+subject,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
		),
	)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	msg := nats.NewMsg(subject)
	msg.Data = data
	// NATS headers are case-sensitive; http.Header canonicalisation would hide traceparent from the consumer.
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		msg.Header.Set(k, v)
	}

	reply, err := c.conn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrUpstreamUnavailable, subject, err)
	}
	return reply.Data, nil
}

func optional(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
