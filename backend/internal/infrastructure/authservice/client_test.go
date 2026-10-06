// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package authservice_test

import (
	"context"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/authservice"
	"github.com/nats-io/nats.go"
)

type stubConn struct {
	replies map[string]string
	err     error
	got     map[string]string
}

func (c *stubConn) RequestMsgWithContext(_ context.Context, msg *nats.Msg) (*nats.Msg, error) {
	if c.got == nil {
		c.got = map[string]string{}
	}
	c.got[msg.Subject] = string(msg.Data)
	if c.err != nil {
		return nil, c.err
	}
	return &nats.Msg{Data: []byte(c.replies[msg.Subject])}, nil
}

const notFound = `{"success":false,"error":"user not found"}`

func TestUsernameByEmail(t *testing.T) {
	conn := &stubConn{replies: map[string]string{"lfx.auth-service.email_to_username": "ada\n"}}
	got, err := authservice.NewClient(conn, 0).UsernameByEmail(context.Background(), "ada@example.org")
	if err != nil || got != "ada" {
		t.Fatalf("got %q, %v; want ada", got, err)
	}
	if conn.got["lfx.auth-service.email_to_username"] != "ada@example.org" {
		t.Errorf("request = %q; want the plain email", conn.got["lfx.auth-service.email_to_username"])
	}
}

func TestAccount(t *testing.T) {
	conn := &stubConn{replies: map[string]string{"lfx.auth-service.user_metadata.read": `{"success":true,"data":{"name":"Ada Lovelace","given_name":"Ada","family_name":"","picture":"https://example.org/ada.png"}}`}}
	got, err := authservice.NewClient(conn, 0).Account(context.Background(), "ada")
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if got.Username != "ada" || *got.Name != "Ada Lovelace" || *got.AvatarURL != "https://example.org/ada.png" || got.FamilyName != nil {
		t.Errorf("got %+v", got)
	}
}

func TestAccount_NoMetadata(t *testing.T) {
	for _, body := range []string{`{"success":true,"data":null}`, `{"success":true}`} {
		conn := &stubConn{replies: map[string]string{"lfx.auth-service.user_metadata.read": body}}
		got, err := authservice.NewClient(conn, 0).Account(context.Background(), "ada")
		if err != nil || got.Username != "ada" || got.Name != nil || got.AvatarURL != nil {
			t.Errorf("%s: got %+v, %v; want ada with an empty profile", body, got, err)
		}
	}
}

func TestPrimaryEmail(t *testing.T) {
	conn := &stubConn{replies: map[string]string{"lfx.auth-service.user_emails.read": `{"success":true,"data":{"primary_email":"ada@example.org","alternate_emails":[]}}`}}
	got, err := authservice.NewClient(conn, 0).PrimaryEmail(context.Background(), "ada")
	if err != nil || got != "ada@example.org" {
		t.Fatalf("got %q, %v; want ada@example.org", got, err)
	}
	if want := `{"user":{"auth_token":"ada"}}`; conn.got["lfx.auth-service.user_emails.read"] != want {
		t.Errorf("request = %s; want %s", conn.got["lfx.auth-service.user_emails.read"], want)
	}
}

func TestErrors(t *testing.T) {
	ctx := context.Background()
	calls := map[string]func(*authservice.Client) error{
		"email_to_username": func(c *authservice.Client) error { _, err := c.UsernameByEmail(ctx, "a@example.org"); return err },
		"user_metadata":     func(c *authservice.Client) error { _, err := c.Account(ctx, "ada"); return err },
		"user_emails":       func(c *authservice.Client) error { _, err := c.PrimaryEmail(ctx, "ada"); return err },
	}
	for name, tc := range map[string]struct {
		conn *stubConn
		want error
	}{
		"not found":  {&stubConn{replies: allReplies(notFound)}, domain.ErrAccountNotFound},
		"failure":    {&stubConn{replies: allReplies(`{"success":false,"error":"management API timeout"}`)}, domain.ErrUpstreamUnavailable},
		"no reply":   {&stubConn{err: nats.ErrNoResponders}, domain.ErrUpstreamUnavailable},
		"empty data": {&stubConn{replies: allReplies(`{"success":true}`)}, domain.ErrUpstreamUnavailable},
	} {
		for call, fn := range calls {
			if name == "empty data" && call == "user_metadata" {
				continue // an account without metadata is valid; see TestAccount_NoMetadata
			}
			if err := fn(authservice.NewClient(tc.conn, 0)); !errors.Is(err, tc.want) {
				t.Errorf("%s/%s: err = %v; want %v", name, call, err, tc.want)
			}
		}
	}
}

func allReplies(body string) map[string]string {
	return map[string]string{
		"lfx.auth-service.email_to_username":  body,
		"lfx.auth-service.user_metadata.read": body,
		"lfx.auth-service.user_emails.read":   body,
	}
}
