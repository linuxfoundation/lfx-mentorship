// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain_test

import (
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

func TestObjectKeyFromLocator(t *testing.T) {
	const prefix = "https://cdn.example.org/mentorship"
	tests := []struct {
		name    string
		bucket  domain.ObjectBucket
		locator string
		prefix  string
		wantKey string
		wantOK  bool
	}{
		{"own logo URL", domain.ObjectBucketLogos, prefix + "/abc-logo.png", prefix, "abc-logo.png", true},
		{"escaped logo key", domain.ObjectBucketLogos, prefix + "/abc-my%20logo.png", prefix, "abc-my logo.png", true},
		{"foreign logo URL", domain.ObjectBucketLogos, "https://lf.example/logo.png", prefix, "", false},
		{"prefix look-alike", domain.ObjectBucketLogos, prefix + "-evil/abc.png", prefix, "", false},
		{"prefix only", domain.ObjectBucketLogos, prefix + "/", prefix, "", false},
		{"no prefix configured", domain.ObjectBucketLogos, prefix + "/abc.png", "", "", false},
		{"private key", domain.ObjectBucketAttachments, "abc-essay.pdf", "", "abc-essay.pdf", true},
		{"legacy URL in private column", domain.ObjectBucketAttachments, "https://legacy.example/x.pdf", "", "", false},
		{"unknown bucket", domain.ObjectBucket("other"), "abc", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := domain.ObjectKeyFromLocator(tc.bucket, tc.locator, tc.prefix)
			if key != tc.wantKey || ok != tc.wantOK {
				t.Fatalf("got (%q, %v); want (%q, %v)", key, ok, tc.wantKey, tc.wantOK)
			}
		})
	}
}
