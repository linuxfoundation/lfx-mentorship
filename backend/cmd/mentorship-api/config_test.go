// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLoadEmailConfig(t *testing.T) {
	valid := map[string]string{
		"PUBLIC_SITE_URL":          "https://mentorship.example.org/",
		"SELF_SERVE_URL":           "https://app.example.org",
		"EMAIL_COPY_INBOX":         "copies@linuxfoundation.org",
		"EMAIL_REVIEW_INBOX":       "review@linuxfoundation.org",
		"EMAIL_ALLOWED_RECIPIENTS": "",
	}
	for name, tc := range map[string]struct {
		nats     bool
		override map[string]string
		wantErr  string
	}{
		"nats unset skips validation": {nats: false, override: map[string]string{"PUBLIC_SITE_URL": "", "EMAIL_COPY_INBOX": "", "EMAIL_REVIEW_INBOX": ""}},
		"valid":                       {nats: true},
		"missing public site":         {nats: true, override: map[string]string{"PUBLIC_SITE_URL": ""}, wantErr: "PUBLIC_SITE_URL"},
		"relative self serve":         {nats: true, override: map[string]string{"SELF_SERVE_URL": "app.example.org"}, wantErr: "SELF_SERVE_URL"},
		"non-http scheme":             {nats: true, override: map[string]string{"PUBLIC_SITE_URL": "javascript://x"}, wantErr: "PUBLIC_SITE_URL"},
		"query on base url":           {nats: true, override: map[string]string{"SELF_SERVE_URL": "https://app.example.org?tenant=x"}, wantErr: "SELF_SERVE_URL"},
		"empty query on base url":     {nats: true, override: map[string]string{"SELF_SERVE_URL": "https://app.example.org?"}, wantErr: "SELF_SERVE_URL"},
		"fragment on base url":        {nats: true, override: map[string]string{"PUBLIC_SITE_URL": "https://mentorship.example.org#top"}, wantErr: "PUBLIC_SITE_URL"},
		"missing copy inbox":          {nats: true, override: map[string]string{"EMAIL_COPY_INBOX": ""}, wantErr: "EMAIL_COPY_INBOX"},
		"missing review inbox":        {nats: true, override: map[string]string{"EMAIL_REVIEW_INBOX": ""}, wantErr: "EMAIL_REVIEW_INBOX"},
		"invalid review inbox":        {nats: true, override: map[string]string{"EMAIL_REVIEW_INBOX": "review"}, wantErr: "EMAIL_REVIEW_INBOX"},
		"review inbox list":           {nats: true, override: map[string]string{"EMAIL_REVIEW_INBOX": " a@linuxfoundation.org, b@contractor.linuxfoundation.org ,"}},
		"review list with bad entry":  {nats: true, override: map[string]string{"EMAIL_REVIEW_INBOX": "a@linuxfoundation.org,review"}, wantErr: "EMAIL_REVIEW_INBOX"},
		"comma-only review inbox":     {nats: true, override: map[string]string{"EMAIL_REVIEW_INBOX": " , "}, wantErr: "EMAIL_REVIEW_INBOX"},
		"allowed recipients":          {nats: true, override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": " a@linuxfoundation.org, b@contractor.linuxfoundation.org ,"}},
		"invalid allowed recipient":   {nats: true, override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": "a@linuxfoundation.org,not-an-address"}, wantErr: "EMAIL_ALLOWED_RECIPIENTS"},
		"comma-only recipients":       {nats: true, override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": " , "}, wantErr: "EMAIL_ALLOWED_RECIPIENTS"},
		"blank recipients":            {nats: true, override: map[string]string{"EMAIL_ALLOWED_RECIPIENTS": " "}, wantErr: "EMAIL_ALLOWED_RECIPIENTS"},
		"copy inbox":                  {nats: true, override: map[string]string{"EMAIL_COPY_INBOX": " copies@linuxfoundation.org "}},
		"display-name copy inbox":     {nats: true, override: map[string]string{"EMAIL_COPY_INBOX": "Copies <copies@linuxfoundation.org>"}, wantErr: "EMAIL_COPY_INBOX"},
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
			wantReview := []string{"review@linuxfoundation.org"}
			if name == "review inbox list" {
				wantReview = []string{"a@linuxfoundation.org", "b@contractor.linuxfoundation.org"}
			}
			if tc.nats && !slices.Equal(cfg.ReviewInboxes, wantReview) {
				t.Fatalf("ReviewInboxes = %q, want %q", cfg.ReviewInboxes, wantReview)
			}
			if want := []string{"a@linuxfoundation.org", "b@contractor.linuxfoundation.org"}; name == "allowed recipients" && !slices.Equal(cfg.AllowedRecipients, want) {
				t.Fatalf("AllowedRecipients = %q, want %q", cfg.AllowedRecipients, want)
			}
			if name == "valid" && cfg.AllowedRecipients != nil {
				t.Fatalf("AllowedRecipients = %q, want nil when unset", cfg.AllowedRecipients)
			}
			if tc.nats && cfg.CopyInbox != "copies@linuxfoundation.org" {
				t.Fatalf("CopyInbox = %q, want the trimmed address", cfg.CopyInbox)
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

func TestLoadConfig(t *testing.T) {
	// Every variable loadConfig reads is pinned, so a developer or CI shell cannot leak in.
	valid := map[string]string{
		"HEIMDALL_JWKS_URL":                    "https://heimdall.example.org/.well-known/jwks",
		"HEIMDALL_JWT_AUDIENCE":                "lfx-mentorship",
		"HEIMDALL_JWT_ISSUER":                  "heimdall",
		"DISABLED_MOCK_LOCAL_PRINCIPAL":        "",
		"ALLOW_MOCK_LOCAL_PRINCIPAL_BYPASS":    "",
		"MENTOR_INVITE_SECRET":                 "secret",
		"PORT":                                 "",
		"DB_MAX_CONNS":                         "",
		"DB_MIN_CONNS":                         "",
		"JWT_CLOCK_SKEW":                       "",
		"FGA_NATS_URL":                         "",
		"FGA_RELAY_BATCH_SIZE":                 "",
		"FGA_RELAY_MAX_ATTEMPTS":               "",
		"FGA_RELAY_INTERVAL":                   "",
		"FGA_RELAY_RETRY_DELAY":                "",
		"FGA_ACCESS_WAIT_TIMEOUT":              "",
		"INDEX_RELAY_RETRY_DELAY":              "",
		"INDEX_RELAY_MAX_ATTEMPTS":             "",
		"INDEXER_SERVICE_TOKEN":                "",
		"CROWDFUNDING_BASE_URL":                "https://crowdfunding.example.org/",
		"CROWDFUNDING_TIMEOUT":                 "",
		"PUBLIC_SITE_URL":                      "",
		"SELF_SERVE_URL":                       "",
		"EMAIL_COPY_INBOX":                     "",
		"EMAIL_REVIEW_INBOX":                   "",
		"EMAIL_ALLOWED_RECIPIENTS":             "",
		"AWS_REGION":                           "",
		"LOGOS_S3_BUCKET":                      "",
		"LOGOS_S3_ENDPOINT_URL":                "",
		"LOGOS_S3_CREATE_MISSING_BUCKET":       "",
		"LOGOS_CDN_URL_PREFIX":                 "",
		"ATTACHMENTS_S3_BUCKET":                "",
		"ATTACHMENTS_S3_ENDPOINT_URL":          "",
		"ATTACHMENTS_S3_CREATE_MISSING_BUCKET": "",
		"ATTACHMENTS_CDN_URL_PREFIX":           "",
		"OTEL_SERVICE_NAME":                    "",
		"OTEL_SERVICE_VERSION":                 "",
		"OTEL_EXPORTER_OTLP_ENDPOINT":          "",
	}
	for name, tc := range map[string]struct {
		override map[string]string
		wantErr  string
	}{
		"defaults": {},
		"mock principal instead of heimdall": {override: map[string]string{
			"HEIMDALL_JWKS_URL": "", "HEIMDALL_JWT_AUDIENCE": "", "HEIMDALL_JWT_ISSUER": "", "DISABLED_MOCK_LOCAL_PRINCIPAL": "localdev",
		}},
		"overrides": {override: map[string]string{
			"PORT": "9090", "DB_MAX_CONNS": "20", "DB_MIN_CONNS": "4", "JWT_CLOCK_SKEW": "30s",
			"FGA_RELAY_BATCH_SIZE": "25", "FGA_RELAY_MAX_ATTEMPTS": "3", "FGA_RELAY_INTERVAL": "2s",
			"FGA_RELAY_RETRY_DELAY": "5m", "INDEX_RELAY_RETRY_DELAY": "3m", "INDEX_RELAY_MAX_ATTEMPTS": "7",
			"CROWDFUNDING_TIMEOUT": "4s", "FGA_ACCESS_WAIT_TIMEOUT": "5s",
		}},
		"access wait off":              {override: map[string]string{"FGA_ACCESS_WAIT_TIMEOUT": "0"}},
		"bad access wait timeout":      {override: map[string]string{"FGA_ACCESS_WAIT_TIMEOUT": "soon"}, wantErr: "FGA_ACCESS_WAIT_TIMEOUT"},
		"access wait timeout over cap": {override: map[string]string{"FGA_ACCESS_WAIT_TIMEOUT": "20s"}, wantErr: "FGA_ACCESS_WAIT_TIMEOUT"},
		"partial heimdall config":      {override: map[string]string{"HEIMDALL_JWT_ISSUER": ""}, wantErr: "must be set together"},
		"no heimdall and no mock":      {override: map[string]string{"HEIMDALL_JWKS_URL": "", "HEIMDALL_JWT_AUDIENCE": "", "HEIMDALL_JWT_ISSUER": ""}, wantErr: "are required"},
		"non-numeric port":             {override: map[string]string{"PORT": "http"}, wantErr: "PORT"},
		"non-numeric max conns":        {override: map[string]string{"DB_MAX_CONNS": "many"}, wantErr: "DB_MAX_CONNS"},
		"non-numeric min conns":        {override: map[string]string{"DB_MIN_CONNS": "few"}, wantErr: "DB_MIN_CONNS"},
		"bad clock skew":               {override: map[string]string{"JWT_CLOCK_SKEW": "5"}, wantErr: "JWT_CLOCK_SKEW"},
		"zero relay batch":             {override: map[string]string{"FGA_RELAY_BATCH_SIZE": "0"}, wantErr: "FGA_RELAY_BATCH_SIZE"},
		"negative relay attempts":      {override: map[string]string{"FGA_RELAY_MAX_ATTEMPTS": "-1"}, wantErr: "FGA_RELAY_MAX_ATTEMPTS"},
		"bad relay interval":           {override: map[string]string{"FGA_RELAY_INTERVAL": "soon"}, wantErr: "FGA_RELAY_INTERVAL"},
		"zero relay interval":          {override: map[string]string{"FGA_RELAY_INTERVAL": "0s"}, wantErr: "FGA_RELAY_INTERVAL"},
		"zero relay retry delay":       {override: map[string]string{"FGA_RELAY_RETRY_DELAY": "0s"}, wantErr: "FGA_RELAY_RETRY_DELAY"},
		"negative index retry delay":   {override: map[string]string{"INDEX_RELAY_RETRY_DELAY": "-1m"}, wantErr: "INDEX_RELAY_RETRY_DELAY"},
		"zero index attempts":          {override: map[string]string{"INDEX_RELAY_MAX_ATTEMPTS": "0"}, wantErr: "INDEX_RELAY_MAX_ATTEMPTS"},
		"bad crowdfunding timeout":     {override: map[string]string{"CROWDFUNDING_TIMEOUT": "ten"}, wantErr: "CROWDFUNDING_TIMEOUT"},
		"zero crowdfunding timeout":    {override: map[string]string{"CROWDFUNDING_TIMEOUT": "0s"}, wantErr: "CROWDFUNDING_TIMEOUT"},
		"email config checked by nats": {override: map[string]string{"FGA_NATS_URL": "nats://nats:4222", "PUBLIC_SITE_URL": ""}, wantErr: "PUBLIC_SITE_URL"},
		"storage config checked":       {override: map[string]string{"LOGOS_S3_BUCKET": "logos"}, wantErr: "AWS_REGION"},
	} {
		t.Run(name, func(t *testing.T) {
			for k, v := range valid {
				t.Setenv(k, v)
			}
			for k, v := range tc.override {
				t.Setenv(k, v)
			}
			cfg, err := loadConfig()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want mention of %s", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if cfg.Crowdfunding.BaseURL != "https://crowdfunding.example.org" || cfg.Local.InviteSecret != "secret" {
				t.Fatalf("crowdfunding base %q, invite secret %q", cfg.Crowdfunding.BaseURL, cfg.Local.InviteSecret)
			}
			switch name {
			case "defaults":
				if cfg.Server.Port != 8080 || cfg.Database.MaxConns != 10 || cfg.Database.MinConns != 2 || cfg.JWT.ClockSkew != 5*time.Second ||
					cfg.FGA.RelayBatch != 50 || cfg.FGA.RelayMaxAttempts != 10 || cfg.FGA.RelayInterval != time.Second || cfg.FGA.RelayRetryDelay != time.Minute ||
					cfg.FGA.AccessWaitTimeout != 10*time.Second ||
					cfg.Indexer.RetryDelay != time.Minute || cfg.Indexer.MaxAttempts != 10 || cfg.Crowdfunding.Timeout != 10*time.Second ||
					cfg.OTel.ServiceName != "lfx-mentorship-api" || cfg.OTel.ServiceVersion != "dev" ||
					cfg.Storage.Logos.CreateMissingBucket || cfg.Storage.Attachments.CreateMissingBucket ||
					cfg.Email.PublicSiteURL != "" || cfg.Email.ReviewInboxes != nil || cfg.Email.AllowedRecipients != nil {
					t.Fatalf("unexpected defaults %+v", cfg)
				}
			case "access wait off":
				if cfg.FGA.AccessWaitTimeout != 0 {
					t.Fatalf("access wait timeout = %s; want 0 (off)", cfg.FGA.AccessWaitTimeout)
				}
			case "mock principal instead of heimdall":
				if cfg.Local.DisabledMockLocalPrincipal != "localdev" || cfg.JWT.HeimdallJWKSURL != "" {
					t.Fatalf("unexpected local config %+v / %+v", cfg.Local, cfg.JWT)
				}
			case "overrides":
				if cfg.Server.Port != 9090 || cfg.Database.MaxConns != 20 || cfg.Database.MinConns != 4 || cfg.JWT.ClockSkew != 30*time.Second ||
					cfg.FGA.RelayBatch != 25 || cfg.FGA.RelayMaxAttempts != 3 || cfg.FGA.RelayInterval != 2*time.Second || cfg.FGA.RelayRetryDelay != 5*time.Minute ||
					cfg.FGA.AccessWaitTimeout != 5*time.Second ||
					cfg.Indexer.RetryDelay != 3*time.Minute || cfg.Indexer.MaxAttempts != 7 || cfg.Crowdfunding.Timeout != 4*time.Second {
					t.Fatalf("overrides not applied %+v", cfg)
				}
			}
		})
	}
}
