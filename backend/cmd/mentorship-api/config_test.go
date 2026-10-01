// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestLoadEmailConfig(t *testing.T) {
	valid := map[string]string{
		"PUBLIC_SITE_URL": "https://mentorship.example.org/",
		"SELF_SERVE_URL":  "https://app.example.org",
		"EMAIL_HR_INBOX":  "hr@linuxfoundation.org",
	}
	for name, tc := range map[string]struct {
		nats     bool
		override map[string]string
		wantErr  string
	}{
		"nats unset skips validation": {nats: false, override: map[string]string{"PUBLIC_SITE_URL": "", "EMAIL_HR_INBOX": ""}},
		"valid":                       {nats: true},
		"missing public site":         {nats: true, override: map[string]string{"PUBLIC_SITE_URL": ""}, wantErr: "PUBLIC_SITE_URL"},
		"relative self serve":         {nats: true, override: map[string]string{"SELF_SERVE_URL": "app.example.org"}, wantErr: "SELF_SERVE_URL"},
		"non-http scheme":             {nats: true, override: map[string]string{"PUBLIC_SITE_URL": "javascript://x"}, wantErr: "PUBLIC_SITE_URL"},
		"query on base url":           {nats: true, override: map[string]string{"SELF_SERVE_URL": "https://app.example.org?tenant=x"}, wantErr: "SELF_SERVE_URL"},
		"empty query on base url":     {nats: true, override: map[string]string{"SELF_SERVE_URL": "https://app.example.org?"}, wantErr: "SELF_SERVE_URL"},
		"fragment on base url":        {nats: true, override: map[string]string{"PUBLIC_SITE_URL": "https://mentorship.example.org#top"}, wantErr: "PUBLIC_SITE_URL"},
		"missing hr inbox":            {nats: true, override: map[string]string{"EMAIL_HR_INBOX": ""}, wantErr: "EMAIL_HR_INBOX"},
		"display-name hr inbox":       {nats: true, override: map[string]string{"EMAIL_HR_INBOX": "HR <hr@linuxfoundation.org>"}, wantErr: "EMAIL_HR_INBOX"},
	} {
		t.Run(name, func(t *testing.T) {
			for k, v := range valid {
				t.Setenv(k, v)
			}
			for k, v := range tc.override {
				t.Setenv(k, v)
			}
			cfg, err := loadEmailConfig(tc.nats)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want mention of %s", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if tc.nats && cfg.PublicSiteURL != "https://mentorship.example.org" {
				t.Fatalf("PublicSiteURL = %q, want trailing slash trimmed", cfg.PublicSiteURL)
			}
		})
	}
}
