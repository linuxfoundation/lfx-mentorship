// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/handler"
)

func TestIndexMetadataPreservesActorHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/programs", nil)
	req.Header.Set("Authorization", "Bearer caller")
	req.Header.Set("X-On-Behalf-Of", "alice")
	var got map[string]string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = domain.IndexHeadersFromContext(r.Context())
	})

	handler.IndexMetadata(next).ServeHTTP(httptest.NewRecorder(), req)

	if got["authorization"] != "Bearer caller" || got["x-on-behalf-of"] != "alice" {
		t.Fatalf("index headers = %v", got)
	}
}
