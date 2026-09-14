// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLedgerClient_GetTransactionsPage_UsesTransactionTypeAndFilters(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s; want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Fatalf("Authorization = %q; want Bearer test-api-key", got)
		}
		checks := map[string]string{
			"startDate":   "0",
			"page":        "2",
			"perPage":     "50",
			"txnType":     "debit",
			"txnCategory": "mentorship",
			"projectID":   "program-1",
		}
		for key, want := range checks {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s = %q; want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hasNext":false,"transactions":[{"projectID":"program-1","txnCategory":"mentorship","amount":-125}]}`))
	}))
	defer server.Close()

	client := NewLedgerClient(LedgerConfig{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
		Timeout: 2 * time.Second,
	})

	page, err := client.GetTransactionsPage(context.Background(), "program-1", "debit", 2, 50)
	if err != nil {
		t.Fatalf("GetTransactionsPage: %v", err)
	}
	if len(page.Transactions) != 1 || page.Transactions[0].Amount != -125 {
		t.Fatalf("transactions = %+v; want one transaction with amount -125", page.Transactions)
	}
}
