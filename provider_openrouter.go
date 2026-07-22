package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

// OpenRouterProvider handles OpenRouter accounts through OpenRouter's
// Anthropic-compatible "Anthropic Skin" API. OpenRouter is an aggregator that
// routes to hundreds of vendor-prefixed models (e.g. "anthropic/claude-opus-4.5",
// "deepseek/deepseek-v4"), so unlike the single-family providers it has no
// fixed model catalog - any request model prefixed "openrouter/" is routed
// here, with the prefix stripped before forwarding upstream.
type OpenRouterProvider struct {
	openrouterBase *url.URL
}

// NewOpenRouterProvider creates a new OpenRouter provider.
func NewOpenRouterProvider(openrouterBase *url.URL) *OpenRouterProvider {
	return &OpenRouterProvider{
		openrouterBase: openrouterBase,
	}
}

func (p *OpenRouterProvider) Type() AccountType {
	return AccountTypeOpenRouter
}

type OpenRouterAuthJSON struct {
	APIKey string `json:"api_key"`
}

func (p *OpenRouterProvider) LoadAccount(name, path string, data []byte) (*ProviderConnection, error) {
	var oj OpenRouterAuthJSON
	if err := json.Unmarshal(data, &oj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if oj.APIKey == "" {
		return nil, nil
	}

	acc := &ProviderConnection{
		Type:        AccountTypeOpenRouter,
		ID:          strings.TrimSuffix(name, filepath.Ext(name)),
		File:        path,
		AccessToken: oj.APIKey,
		PlanType:    "openrouter",
	}
	return acc, nil
}

func (p *OpenRouterProvider) SetAuthHeaders(req *http.Request, acc *ProviderConnection) {
	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
}

func (p *OpenRouterProvider) RefreshToken(ctx context.Context, acc *ProviderConnection, transport http.RoundTripper) error {
	return nil
}

func (p *OpenRouterProvider) ParseUsage(obj map[string]any) *RequestUsage {
	return anthropicMessagesEngine.ParseUsage(obj)
}

func (p *OpenRouterProvider) ParseUsageHeaders(acc *ProviderConnection, headers http.Header) {
	// OpenRouter's Anthropic-compatible endpoint does not currently expose quota headers.
}

func (p *OpenRouterProvider) UpstreamURL(path string) *url.URL {
	return p.openrouterBase
}

func (p *OpenRouterProvider) MatchesPath(path string) bool {
	// OpenRouter is model-routed.
	return false
}

func (p *OpenRouterProvider) NormalizePath(path string) string {
	return path
}

func (p *OpenRouterProvider) DetectsSSE(path string, contentType string) bool {
	return eventStreamDetector.Detect(path, contentType)
}

const openrouterModelPrefix = "openrouter/"

// isOpenRouterModel reports whether a request model should route to a pooled
// OpenRouter account - identified by an explicit "openrouter/" prefix rather
// than a fixed catalog, since OpenRouter routes to any vendor-prefixed model.
func isOpenRouterModel(model string) bool {
	return strings.HasPrefix(strings.TrimSpace(model), openrouterModelPrefix)
}

// openrouterCanonicalModel strips the "openrouter/" prefix, leaving the
// vendor-prefixed model id (e.g. "anthropic/claude-opus-4.5") to forward upstream.
func openrouterCanonicalModel(model string) string {
	return strings.TrimPrefix(strings.TrimSpace(model), openrouterModelPrefix)
}
