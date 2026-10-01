// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
)

func TestCrowdfundingClient_GetCategorizedTransactions_UsesGatewayPathAnonymously(t *testing.T) {
	t.Parallel()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crowdfunding/initiatives/initiative-1/transactions" {
			t.Fatalf("path = %q; want the gateway route without a version segment", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q; want no credentials on the public route", got)
		}
		if got := r.URL.Query().Get("type"); got != "donations" {
			t.Fatalf("type = %q; want donations (required with categoryType)", got)
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
		BaseURL: apiServer.URL + "/crowdfunding/",
		Timeout: 2 * time.Second,
	})

	if _, err := client.GetCategorizedTransactions(context.Background(), "initiative-1", "mentorship", true, 20, 5); err != nil {
		t.Fatalf("GetCategorizedTransactions: %v", err)
	}
}

func TestCrowdfundingClient_GetCategorizedTransactions_NonOKIsUpstreamUnavailable(t *testing.T) {
	t.Parallel()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer apiServer.Close()

	client := NewCrowdfundingClient(CrowdfundingConfig{
		BaseURL: apiServer.URL,
		Timeout: 2 * time.Second,
	})

	_, err := client.GetCategorizedTransactions(context.Background(), "initiative-1", "", false, 10, 0)
	if !errors.Is(err, domain.ErrUpstreamUnavailable) {
		t.Fatalf("err = %v; want ErrUpstreamUnavailable", err)
	}
}

func TestCrowdfundingClient_GetCategorizedTransactions_FallbackDataArray(t *testing.T) {
	t.Parallel()

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
		BaseURL: apiServer.URL,
		Timeout: 2 * time.Second,
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
