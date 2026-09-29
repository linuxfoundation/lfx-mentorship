// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package clients provides outbound HTTP clients for external services.
package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

// CrowdfundingConfig holds outbound crowdfunding API settings.
type CrowdfundingConfig struct {
	BaseURL string
	Timeout time.Duration
}

// CrowdfundingClient fetches categorized transactions from crowdfunding.
type CrowdfundingClient interface {
	GetCategorizedTransactions(ctx context.Context, initiativeID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error)
}

type crowdfundingHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewCrowdfundingClient creates an HTTP client for crowdfunding endpoints.
func NewCrowdfundingClient(cfg CrowdfundingConfig) CrowdfundingClient {
	return &crowdfundingHTTPClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *crowdfundingHTTPClient) GetCategorizedTransactions(ctx context.Context, initiativeID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
	q := url.Values{}
	if strings.TrimSpace(categoryType) != "" {
		q.Set("categoryType", categoryType)
	}
	if subscriptionOnly {
		q.Set("subscriptionOnly", "true")
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset >= 0 {
		q.Set("offset", fmt.Sprintf("%d", offset))
	}

	// BaseURL is the gateway prefix, e.g. https://lfx-api.<domain>/crowdfunding; the gateway routes carry no version segment.
	endpoint := fmt.Sprintf("%s/initiatives/%s/transactions?%s", c.baseURL, url.PathEscape(initiativeID), q.Encode())
	// Crowdfunding serves initiative transactions on its public (anonymous) gateway rule, so no token is sent.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create crowdfunding transactions request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crowdfunding transactions request: %w: %w", err, domain.ErrUpstreamUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("crowdfunding transactions status %d: %s: %w", resp.StatusCode, strings.TrimSpace(string(body)), domain.ErrUpstreamUnavailable)
	}

	var wire struct {
		IndividualTransactions   []models.ProgramTransaction `json:"individual_transactions"`
		OrganizationTransactions []models.ProgramTransaction `json:"organization_transactions"`
		Data                     []models.ProgramTransaction `json:"data"`
		TotalCount               int                         `json:"total_count"`
		Limit                    int                         `json:"limit"`
		Offset                   int                         `json:"offset"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return nil, fmt.Errorf("decode crowdfunding categorized transactions: %w: %w", err, domain.ErrUpstreamUnavailable)
	}

	out := models.ProgramCategorizedTransactions{
		IndividualTransactions:   wire.IndividualTransactions,
		OrganizationTransactions: wire.OrganizationTransactions,
		TotalCount:               wire.TotalCount,
		Limit:                    wire.Limit,
		Offset:                   wire.Offset,
	}

	// Backward/forward compatibility: some crowdfunding environments return a flat
	// "data" array and require local categorization by donor_type.
	if len(out.IndividualTransactions) == 0 && len(out.OrganizationTransactions) == 0 && len(wire.Data) > 0 {
		for _, txn := range wire.Data {
			if strings.EqualFold(strings.TrimSpace(txn.DonorType), "organization") {
				out.OrganizationTransactions = append(out.OrganizationTransactions, txn)
				continue
			}
			out.IndividualTransactions = append(out.IndividualTransactions, txn)
		}
	}

	if out.IndividualTransactions == nil {
		out.IndividualTransactions = []models.ProgramTransaction{}
	}
	if out.OrganizationTransactions == nil {
		out.OrganizationTransactions = []models.ProgramTransaction{}
	}
	return &out, nil
}
