// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"slices"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	base := map[string]string{
		"FGA_NATS_URL":             "nats://nats:4222",
		"SELF_SERVE_URL":           "https://app.example.org/",
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
			if !slices.Equal(cfg.AllowedRecipients, tc.wantTo) {
				t.Errorf("AllowedRecipients = %v; want %v", cfg.AllowedRecipients, tc.wantTo)
			}
		})
	}
}
