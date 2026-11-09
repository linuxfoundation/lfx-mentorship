// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package clients

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestCrowdfundingClient_GetCategorizedTransactions_NotFoundIsInitiativeNotFound(t *testing.T) {
	t.Parallel()

	// A missing initiative is a normal answer, so its connection must stay reusable: net/http reuses
	// one only when the body is read to EOF and closed. Counting server connections instead races
	// the transport returning the connection to its pool.
	body := &trackedBody{Reader: strings.NewReader(`{"error":"not found"}`)}
	client := NewCrowdfundingClient(CrowdfundingConfig{BaseURL: "http://crowdfunding.test", Timeout: 2 * time.Second}).(*crowdfundingHTTPClient)
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: body, Header: http.Header{}, Request: r}, nil
	})

	_, err := client.GetCategorizedTransactions(context.Background(), "initiative-1", "", false, 10, 0)
	if !errors.Is(err, domain.ErrInitiativeNotFound) {
		t.Fatalf("err = %v; want ErrInitiativeNotFound", err)
	}
	if !body.eof || !body.closed {
		t.Fatalf("body read to EOF = %v, closed = %v; want both so the connection is reused", body.eof, body.closed)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// trackedBody records whether a response body was read to EOF and closed.
type trackedBody struct {
	io.Reader
	eof, closed bool
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if errors.Is(err, io.EOF) {
		b.eof = true
	}
	return n, err
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

func TestCrowdfundingClient_GetCategorizedTransactions_NonOKIsUpstreamUnavailable(t *testing.T) {
	t.Parallel()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
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
