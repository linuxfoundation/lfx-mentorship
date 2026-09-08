// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCrowdfundingClient_GetCategorizedTransactions_UsesM2MTokenAndQuery(t *testing.T) {
	t.Parallel()

	var tokenCalls int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s; want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.PostForm.Get("grant_type") != "client_credentials" {
			t.Fatalf("grant_type = %q; want client_credentials", r.PostForm.Get("grant_type"))
		}
		atomic.AddInt32(&tokenCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q; want Bearer test-token", got)
		}
		if got := r.URL.Query().Get("categoryType"); got != "mentorship" {
			t.Fatalf("categoryType = %q; want mentorship", got)
		}
		if got := r.URL.Query().Get("subscriptionOnly"); got != "true" {
			t.Fatalf("subscriptionOnly = %q; want true", got)
		}
		if got := r.URL.Query().Get("limit"); got != "20" {
			t.Fatalf("limit = %q; want 20", got)
		}
		if got := r.URL.Query().Get("offset"); got != "5" {
			t.Fatalf("offset = %q; want 5", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"individual_transactions":[],"organization_transactions":[],"total_count":0,"limit":20,"offset":5}`))
	}))
	defer apiServer.Close()

	client := NewCrowdfundingClient(CrowdfundingConfig{
		BaseURL:      apiServer.URL,
		TokenURL:     tokenServer.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Audience:     "https://api.example",
		Scope:        "access:manage",
		Timeout:      2 * time.Second,
	})

	if _, err := client.GetCategorizedTransactions(context.Background(), "initiative-1", "mentorship", true, 20, 5); err != nil {
		t.Fatalf("GetCategorizedTransactions: %v", err)
	}
	if atomic.LoadInt32(&tokenCalls) != 1 {
		t.Fatalf("tokenCalls = %d; want 1", tokenCalls)
	}
}

func TestCrowdfundingClient_CachesToken(t *testing.T) {
	t.Parallel()

	var tokenCalls int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&tokenCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"cached-token","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"individual_transactions":[],"organization_transactions":[],"total_count":0,"limit":10,"offset":0}`))
	}))
	defer apiServer.Close()

	client := NewCrowdfundingClient(CrowdfundingConfig{
		BaseURL:      apiServer.URL,
		TokenURL:     tokenServer.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Audience:     "https://api.example",
		Timeout:      2 * time.Second,
	})

	if _, err := client.GetCategorizedTransactions(context.Background(), "initiative-1", "mentorship", false, 10, 0); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := client.GetCategorizedTransactions(context.Background(), "initiative-1", "mentorship", false, 10, 0); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if atomic.LoadInt32(&tokenCalls) != 1 {
		t.Fatalf("tokenCalls = %d; want 1", tokenCalls)
	}
}

func TestCrowdfundingClient_GetCategorizedTransactions_FallbackDataArray(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{"id":"t1","amount_cents":1000,"donor_type":"individual","donor_name":"Alice"},
				{"id":"t2","amount_cents":2500,"donor_type":"organization","donor_name":"Google"}
			],
			"total_count": 2,
			"limit": 10,
			"offset": 0
		}`))
	}))
	defer apiServer.Close()

	client := NewCrowdfundingClient(CrowdfundingConfig{
		BaseURL:      apiServer.URL,
		TokenURL:     tokenServer.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		Audience:     "https://api.example",
		Timeout:      2 * time.Second,
	})

	out, err := client.GetCategorizedTransactions(context.Background(), "initiative-1", "mentorship", false, 10, 0)
	if err != nil {
		t.Fatalf("GetCategorizedTransactions: %v", err)
	}

	if len(out.IndividualTransactions) != 1 {
		t.Fatalf("individual len = %d; want 1", len(out.IndividualTransactions))
	}
	if len(out.OrganizationTransactions) != 1 {
		t.Fatalf("organization len = %d; want 1", len(out.OrganizationTransactions))
	}
	if out.IndividualTransactions[0].DonorName != "Alice" {
		t.Fatalf("individual donor = %q; want Alice", out.IndividualTransactions[0].DonorName)
	}
	if out.OrganizationTransactions[0].DonorName != "Google" {
		t.Fatalf("organization donor = %q; want Google", out.OrganizationTransactions[0].DonorName)
	}
}
