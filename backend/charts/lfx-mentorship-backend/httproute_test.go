// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package lfxmentorshipbackend_test

import (
	"os"
	"strings"
	"testing"
)

// Buffering also buffers responses, so it must stay off the catch-all rule that serves downloads.
func TestUploadBufferingIsAttachedToUploadRulesOnly(t *testing.T) {
	contents, err := os.ReadFile("templates/httproute.yaml")
	if err != nil {
		t.Fatalf("read HTTPRoute: %v", err)
	}
	route := string(contents)
	catchAll := strings.Index(route, "type: PathPrefix")
	buffering := strings.Index(route, "-upload-buffering")
	if catchAll < 0 || buffering < 0 || strings.Count(route, "-upload-buffering") != 1 || buffering > catchAll {
		t.Fatalf("upload buffering must be referenced once, by the upload rule ahead of the catch-all rule:\n%s", route)
	}
	for _, path := range []string{"logo-upload", "file-upload"} {
		if !strings.Contains(route[:catchAll], path) {
			t.Errorf("upload rule is missing %q", path)
		}
	}
	if strings.Contains(route[:catchAll], "-download") {
		t.Error("a download route must not be buffered")
	}
}
