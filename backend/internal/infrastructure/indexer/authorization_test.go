// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package indexer

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestManagedAuthorizationProviderCachesToken(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("audience") != "audience" || r.Form.Get("scope") != "scope" {
			t.Fatalf("form=%v", r.Form)
		}
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
	if first != "Bearer token-1" || second != first || atomic.LoadInt32(&calls) != 1 {
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

func TestManagedAuthorizationProviderSendsClientCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := url.ParseQuery(readRequestBody(t, r))
		if err != nil {
			t.Fatal(err)
		}
		if body.Get("client_id") != "client" || body.Get("client_secret") != "secret" {
			t.Fatalf("credentials missing from form")
		}
		_, _ = w.Write([]byte(`{"access_token":"token-1","expires_in":300}`))
	}))
	defer server.Close()

	provider := NewManagedAuthorizationProvider(nil, server.URL, "client", "secret", "audience", "")
	if _, err := provider.Authorization(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func readRequestBody(t *testing.T, r *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
