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
	"sync"
	"time"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

const defaultCrowdfundingScope = "access:manage"

// CrowdfundingConfig holds outbound crowdfunding API and M2M auth settings.
type CrowdfundingConfig struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	Audience     string
	Scope        string
	Timeout      time.Duration
}

// CrowdfundingClient fetches categorized transactions from crowdfunding.
type CrowdfundingClient interface {
	GetCategorizedTransactions(ctx context.Context, initiativeID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error)
}

type crowdfundingHTTPClient struct {
	baseURL      string
	tokenURL     string
	clientID     string
	clientSecret string
	audience     string
	scope        string
	httpClient   *http.Client

	tokenMu     sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewCrowdfundingClient creates an HTTP client for crowdfunding endpoints.
func NewCrowdfundingClient(cfg CrowdfundingConfig) CrowdfundingClient {
	scope := strings.TrimSpace(cfg.Scope)
	if scope == "" {
		scope = defaultCrowdfundingScope
	}
	return &crowdfundingHTTPClient{
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		tokenURL:     cfg.TokenURL,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		audience:     cfg.Audience,
		scope:        scope,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (c *crowdfundingHTTPClient) GetCategorizedTransactions(ctx context.Context, initiativeID, categoryType string, subscriptionOnly bool, limit, offset int) (*models.ProgramCategorizedTransactions, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

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

	endpoint := fmt.Sprintf("%s/v1/initiatives/%s/transactions?%s", c.baseURL, url.PathEscape(initiativeID), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create crowdfunding transactions request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crowdfunding transactions request: %w: %w", err, domain.ErrUpstreamUnavailable)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("crowdfunding transactions status %d: %s: %w", resp.StatusCode, strings.TrimSpace(string(body)), domain.ErrUpstreamUnavailable)
	}

	var out models.ProgramCategorizedTransactions
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode crowdfunding categorized transactions: %w: %w", err, domain.ErrUpstreamUnavailable)
	}
	if out.IndividualTransactions == nil {
		out.IndividualTransactions = []models.ProgramTransaction{}
	}
	if out.OrganizationTransactions == nil {
		out.OrganizationTransactions = []models.ProgramTransaction{}
	}
	return &out, nil
}

func (c *crowdfundingHTTPClient) getAccessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	now := time.Now()
	if c.accessToken != "" && now.Before(c.expiresAt) {
		return c.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("audience", c.audience)
	if c.scope != "" {
		form.Set("scope", c.scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request m2m token: %w: %w", err, domain.ErrUpstreamUnavailable)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("m2m token status %d: %s: %w", resp.StatusCode, strings.TrimSpace(string(body)), domain.ErrUpstreamUnavailable)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode m2m token response: %w: %w", err, domain.ErrUpstreamUnavailable)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("m2m token response missing access_token: %w", domain.ErrUpstreamUnavailable)
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 60
	}
	refreshSkew := 30
	if expiresIn <= refreshSkew {
		refreshSkew = 1
	}
	c.accessToken = tokenResp.AccessToken
	c.expiresAt = now.Add(time.Duration(expiresIn-refreshSkew) * time.Second)
	return c.accessToken, nil
}
