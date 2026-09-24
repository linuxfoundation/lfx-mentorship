// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ManagedAuthorizationProvider obtains and caches a Heimdall-compatible M2M token.
type ManagedAuthorizationProvider struct {
	client       *http.Client
	tokenURL     string
	clientID     string
	clientSecret string
	audience     string
	scope        string

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewManagedAuthorizationProvider(client *http.Client, tokenURL, clientID, clientSecret, audience, scope string) *ManagedAuthorizationProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &ManagedAuthorizationProvider{client: client, tokenURL: tokenURL, clientID: clientID, clientSecret: clientSecret, audience: audience, scope: scope}
}

func (p *ManagedAuthorizationProvider) Authorization(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Now().Before(p.expiresAt) {
		return p.token, nil
	}
	if p.tokenURL == "" || p.clientID == "" || p.clientSecret == "" || p.audience == "" {
		return "", fmt.Errorf("indexer token configuration is incomplete")
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("audience", p.audience)
	if p.scope != "" {
		form.Set("scope", p.scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create indexer token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request indexer token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("indexer token endpoint returned status %d", resp.StatusCode)
	}
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("decode indexer token response: %w", err)
	}
	if tokenResponse.AccessToken == "" {
		return "", fmt.Errorf("indexer token response missing access_token")
	}
	expiresIn := time.Duration(tokenResponse.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 5 * time.Minute
	}
	p.token = tokenResponse.AccessToken
	p.expiresAt = time.Now().Add(expiresIn - time.Minute)
	return p.token, nil
}
