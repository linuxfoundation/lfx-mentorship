// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package indexer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestManagedAuthorizationProviderCachesToken(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"access_token":"token-1","expires_in":3600}`))
	}))
	defer server.Close()

	provider := NewManagedAuthorizationProvider(nil, server.URL, "client", "secret", "audience", "scope")
	first, err := provider.Authorization(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Authorization(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first != "token-1" || second != first || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("tokens=%q/%q calls=%d", first, second, calls)
	}
}

func TestManagedAuthorizationProviderRejectsMissingToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"expires_in":3600}`))
	}))
	defer server.Close()

	provider := NewManagedAuthorizationProvider(nil, server.URL, "client", "secret", "audience", "scope")
	if _, err := provider.Authorization(context.Background()); err == nil {
		t.Fatal("expected missing access token error")
	}
}
