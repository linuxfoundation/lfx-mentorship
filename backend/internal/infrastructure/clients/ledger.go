// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package clients provides outbound HTTP clients for external services.
package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// LedgerConfig holds Ledger service connection settings.
type LedgerConfig struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// LedgerTransaction is the minimal wire model needed for funding stats sync.
type LedgerTransaction struct {
	ProjectID   string `json:"projectID"`
	TxnCategory models.MentorshipCategoryType `json:"txnCategory"`
	Amount      int64  `json:"amount"`
}

// LedgerTransactionsPage is one paginated /transactions response.
type LedgerTransactionsPage struct {
	HasNext      bool                `json:"hasNext"`
	Transactions []LedgerTransaction `json:"transactions"`
}

// LedgerClient fetches data from the Ledger HTTP API.
type LedgerClient interface {
	GetTransactionsPage(ctx context.Context, projectID string, page int, perPage int) (*LedgerTransactionsPage, error)
}

type ledgerHTTPClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewLedgerClient creates an HTTP client for Ledger service endpoints.
func NewLedgerClient(cfg LedgerConfig) LedgerClient {
	return &ledgerHTTPClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// GetTransactionsPage returns one filtered /transactions page for mentorship credits.
func (c *ledgerHTTPClient) GetTransactionsPage(ctx context.Context, projectID string, page int, perPage int) (*LedgerTransactionsPage, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 100
	}
	if perPage > 100 {
		perPage = 100
	}

	q := url.Values{}
	q.Set("startDate", "0")
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("perPage", fmt.Sprintf("%d", perPage))
	q.Set("txnType", "credit")
	q.Set("txnCategory", string(models.MentorshipCategory))
	if strings.TrimSpace(projectID) != "" {
		q.Set("projectID", projectID)
	}

	endpoint := fmt.Sprintf("%s/transactions?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request transactions page: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ledger GET /transactions returned status %d", resp.StatusCode)
	}

	var out LedgerTransactionsPage
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode transactions response: %w", err)
	}
	if out.Transactions == nil {
		out.Transactions = []LedgerTransaction{}
	}
	return &out, nil
}
