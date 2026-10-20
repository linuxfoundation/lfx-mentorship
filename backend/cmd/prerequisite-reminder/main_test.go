// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSendWithRetry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	errDown := errors.New("database down")
	// failing returns a send that fails its first n calls and then reminds 7, counting every call.
	failing := func(n int, calls *int) func(context.Context) (int, error) {
		return func(context.Context) (int, error) {
			*calls++
			if *calls <= n {
				return 0, errDown
			}
			return 7, nil
		}
	}

	t.Run("succeeds after a failure", func(t *testing.T) {
		calls := 0
		reminded, err := sendWithRetry(context.Background(), logger, 0, failing(1, &calls))
		if err != nil || reminded != 7 || calls != 2 {
			t.Errorf("got (%d, %v) after %d calls; want (7, nil) after 2", reminded, err, calls)
		}
	})
	t.Run("gives up after the last attempt", func(t *testing.T) {
		calls := 0
		_, err := sendWithRetry(context.Background(), logger, 0, failing(sendAttempts, &calls))
		if !errors.Is(err, errDown) || calls != sendAttempts {
			t.Errorf("got %v after %d calls; want %v after %d", err, calls, errDown, sendAttempts)
		}
	})
	t.Run("stops waiting when cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		_, err := sendWithRetry(ctx, logger, time.Hour, failing(sendAttempts, &calls))
		if !errors.Is(err, errDown) || calls != 1 {
			t.Errorf("got %v after %d calls; want %v after 1", err, calls, errDown)
		}
	})
}

func TestLoadConfig(t *testing.T) {
	base := map[string]string{
		"FGA_NATS_URL":             "nats://nats:4222",
		"SELF_SERVE_URL":           "https://app.example.org/",
		"EMAIL_COPY_INBOX":         " copies@linuxfoundation.org ",
		"EMAIL_ALLOWED_RECIPIENTS": "",
	}
	cases := map[string]struct {
		override map[string]string
		wantErr  string
		wantTo   []string
	}{
		"valid":                     {},
		"missing nats":              {override: map[string]string{"FGA_NATS_URL": ""}, wantErr: "FGA_NATS_URL"},
		"missing self serve":        {override: map[string]string{"SELF_SERVE_URL": ""}, wantErr: "SELF_SERVE_URL"},
		"relative self serve":       {override: map[string]string{"SELF_SERVE_URL": "app.example.org"}, wantErr: "SELF_SERVE_URL"},
		"missing copy inbox":        {override: map[string]string{"EMAIL_COPY_INBOX": ""}, wantErr: "EMAIL_COPY_INBOX"},
		"display-name copy inbox":   {override: map[string]string{"EMAIL_COPY_INBOX": "Copies <copies@linuxfoundation.org>"}, wantErr: "EMAIL_COPY_INBOX"},
		"allowed recipients":        {override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": " a@linuxfoundation.org, b@linuxfoundation.org ,"}, wantTo: []string{"a@linuxfoundation.org", "b@linuxfoundation.org"}},
		"invalid allowed recipient": {override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": "a@linuxfoundation.org,nobody"}, wantErr: "EMAIL_ALLOWED_RECIPIENTS"},
		"comma-only recipients":     {override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": " , "}, wantErr: "EMAIL_ALLOWED_RECIPIENTS"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range base {
				if o, ok := tc.override[k]; ok {
					v = o
				}
				t.Setenv(k, v)
			}
			cfg, err := loadConfig()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v; want mention of %s", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadConfig: %v", err)
			}
			if cfg.SelfServeURL != "https://app.example.org" {
				t.Errorf("SelfServeURL = %q; want the trailing slash trimmed", cfg.SelfServeURL)
			}
			if cfg.CopyInbox != "copies@linuxfoundation.org" {
				t.Errorf("CopyInbox = %q; want the trimmed address", cfg.CopyInbox)
			}
			if !slices.Equal(cfg.AllowedRecipients, tc.wantTo) {
				t.Errorf("AllowedRecipients = %v; want %v", cfg.AllowedRecipients, tc.wantTo)
			}
		})
	}
}
