// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"slices"
	"strings"
	"testing"
)

func TestLoadEmailConfig(t *testing.T) {
	valid := map[string]string{
		"PUBLIC_SITE_URL":          "https://mentorship.example.org/",
		"SELF_SERVE_URL":           "https://app.example.org",
		"EMAIL_HR_INBOX":           "hr@linuxfoundation.org",
		"EMAIL_ALLOWED_RECIPIENTS": "",
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
		"allowed recipients":          {nats: true, override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": " a@linuxfoundation.org, b@contractor.linuxfoundation.org ,"}},
		"invalid allowed recipient":   {nats: true, override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": "a@linuxfoundation.org,not-an-address"}, wantErr: "EMAIL_ALLOWED_RECIPIENTS"},
		"comma-only recipients":       {nats: true, override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": " , "}, wantErr: "EMAIL_ALLOWED_RECIPIENTS"},
		"blank recipients":            {nats: true, override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": " "}, wantErr: "EMAIL_ALLOWED_RECIPIENTS"},
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
			if want := []string{"a@linuxfoundation.org", "b@contractor.linuxfoundation.org"}; name == "allowed recipients" && !slices.Equal(cfg.AllowedRecipients, want) {
				t.Fatalf("AllowedRecipients = %q, want %q", cfg.AllowedRecipients, want)
			}
			if name == "valid" && cfg.AllowedRecipients != nil {
				t.Fatalf("AllowedRecipients = %q, want nil when unset", cfg.AllowedRecipients)
			}
		})
	}
}

func TestLoadStorageConfig(t *testing.T) {
	valid := map[string]string{
		"AWS_REGION":                     "us-west-2",
		"LOGOS_S3_BUCKET":                "logos",
		"LOGOS_S3_ENDPOINT_URL":          "http://localhost:5222",
		"LOGOS_S3_CREATE_MISSING_BUCKET": "true",
		"LOGOS_CDN_URL_PREFIX":           "https://cdn.example.org/",
		"ATTACHMENTS_S3_BUCKET":          "attachments",
	}
	for name, tc := range map[string]struct {
		override map[string]string
		wantErr  string
	}{
		"valid":                           {},
		"no buckets needs nothing":        {override: map[string]string{"AWS_REGION": "", "LOGOS_S3_BUCKET": "", "ATTACHMENTS_S3_BUCKET": "", "LOGOS_CDN_URL_PREFIX": ""}},
		"bucket without region":           {override: map[string]string{"AWS_REGION": ""}, wantErr: "AWS_REGION"},
		"attachments only without region": {override: map[string]string{"AWS_REGION": "", "LOGOS_S3_BUCKET": ""}, wantErr: "AWS_REGION"},
		"attachments share logos bucket":  {override: map[string]string{"ATTACHMENTS_S3_BUCKET": "logos"}, wantErr: "ATTACHMENTS_S3_BUCKET"},
		"logos without cdn prefix":        {override: map[string]string{"LOGOS_CDN_URL_PREFIX": ""}, wantErr: "LOGOS_CDN_URL_PREFIX"},
		"relative cdn prefix":             {override: map[string]string{"LOGOS_CDN_URL_PREFIX": "cdn.example.org"}, wantErr: "LOGOS_CDN_URL_PREFIX"},
		"cdn prefix with query":           {override: map[string]string{"LOGOS_CDN_URL_PREFIX": "https://cdn.example.org?v=1"}, wantErr: "LOGOS_CDN_URL_PREFIX"},
		"bad create flag":                 {override: map[string]string{"ATTACHMENTS_S3_CREATE_MISSING_BUCKET": "yes please"}, wantErr: "ATTACHMENTS_S3_CREATE_MISSING_BUCKET"},
	} {
		t.Run(name, func(t *testing.T) {
			for k, v := range valid {
				t.Setenv(k, v)
			}
			for k, v := range tc.override {
				t.Setenv(k, v)
			}
			cfg, err := loadStorageConfig()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want mention of %s", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if name == "valid" && (cfg.Logos.CDNURLPrefix != "https://cdn.example.org" || !cfg.Logos.CreateMissingBucket || cfg.Attachments.CreateMissingBucket) {
				t.Fatalf("unexpected config %+v", cfg)
			}
		})
	}
}
