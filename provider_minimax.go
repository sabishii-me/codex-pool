package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// MinimaxProvider handles MiniMax API accounts.
type MinimaxProvider struct {
	minimaxBase *url.URL
}

// NewMinimaxProvider creates a new MiniMax provider.
func NewMinimaxProvider(minimaxBase *url.URL) *MinimaxProvider {
	return &MinimaxProvider{
		minimaxBase: minimaxBase,
	}
}

func (p *MinimaxProvider) Type() AccountType {
	return AccountTypeMinimax
}

// MinimaxAuthJSON is the format for MiniMax auth files.
type MinimaxAuthJSON struct {
	APIKey string `json:"api_key"`
}

func (p *MinimaxProvider) LoadAccount(name, path string, data []byte) (*ProviderConnection, error) {
	var mj MinimaxAuthJSON
	if err := json.Unmarshal(data, &mj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if mj.APIKey == "" {
		return nil, nil
	}

	acc := &ProviderConnection{
		Type:        AccountTypeMinimax,
		ID:          strings.TrimSuffix(name, filepath.Ext(name)),
		File:        path,
		AccessToken: mj.APIKey,
		PlanType:    "minimax",
	}
	return acc, nil
}

func (p *MinimaxProvider) SetAuthHeaders(req *http.Request, acc *ProviderConnection) {
	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
}

func (p *MinimaxProvider) RefreshToken(ctx context.Context, acc *ProviderConnection, transport http.RoundTripper) error {
	return nil
}

func (p *MinimaxProvider) ParseUsage(obj map[string]any) *RequestUsage {
	return anthropicMessagesEngine.ParseUsage(obj)
}

func (p *MinimaxProvider) ParseUsageHeaders(acc *ProviderConnection, headers http.Header) {
	applyMinimaxRateLimits(acc, headers, time.Now())
}

func (p *MinimaxProvider) UpstreamURL(path string) *url.URL {
	return p.minimaxBase
}

func (p *MinimaxProvider) MatchesPath(path string) bool {
	// MiniMax is routed by model name, not by path.
	return false
}

func (p *MinimaxProvider) NormalizePath(path string) string {
	return path
}

func (p *MinimaxProvider) DetectsSSE(path string, contentType string) bool {
	return eventStreamDetector.Detect(path, contentType)
}

// minimaxModels maps request model names to the canonical model name sent upstream.
// isMinimaxModel returns true if the given model name should be routed to MiniMax.
func isMinimaxModel(model string) bool {
	_, ok := modelForProvider(AccountTypeMinimax, model)
	return ok
}

// minimaxCanonicalModel returns the canonical upstream model name for a MiniMax alias.
func minimaxCanonicalModel(model string) string {
	if found, ok := modelForProvider(AccountTypeMinimax, model); ok {
		return found.ID
	}
	return model
}
