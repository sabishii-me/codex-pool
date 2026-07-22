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

// KimiProvider handles Kimi "for Coding" plan accounts (api.kimi.com/coding) -
// a distinct product/key from Moonshot's general Kimi Platform API.
type KimiProvider struct {
	kimiBase *url.URL
}

// NewKimiProvider creates a new Kimi provider.
func NewKimiProvider(kimiBase *url.URL) *KimiProvider {
	return &KimiProvider{
		kimiBase: kimiBase,
	}
}

func (p *KimiProvider) Type() AccountType {
	return AccountTypeKimi
}

// KimiAuthJSON is the format for Kimi auth files.
type KimiAuthJSON struct {
	APIKey string `json:"api_key"`
}

func (p *KimiProvider) LoadAccount(name, path string, data []byte) (*ProviderConnection, error) {
	var kj KimiAuthJSON
	if err := json.Unmarshal(data, &kj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if kj.APIKey == "" {
		return nil, nil
	}

	acc := &ProviderConnection{
		Type:        AccountTypeKimi,
		ID:          strings.TrimSuffix(name, filepath.Ext(name)),
		File:        path,
		AccessToken: kj.APIKey,
		PlanType:    "kimi",
	}
	return acc, nil
}

func (p *KimiProvider) SetAuthHeaders(req *http.Request, acc *ProviderConnection) {
	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
}

func (p *KimiProvider) RefreshToken(ctx context.Context, acc *ProviderConnection, transport http.RoundTripper) error {
	// API keys don't need refresh
	return nil
}

func (p *KimiProvider) ParseUsage(obj map[string]any) *RequestUsage {
	if usage := anthropicMessagesEngine.ParseUsage(obj); usage != nil {
		return usage
	}
	// Kimi proxies Anthropic/OpenAI-style responses, so parse both formats.
	return openAIChatLegacyKimiEngine.ParseUsage(obj)
}

func (p *KimiProvider) ParseUsageHeaders(acc *ProviderConnection, headers http.Header) {
	snap, ok := parseKimiResponseRateLimits(headers)
	if !ok {
		return
	}
	acc.mu.Lock()
	acc.Usage = mergeUsage(acc.Usage, snap)
	acc.mu.Unlock()
}

func (p *KimiProvider) UpstreamURL(path string) *url.URL {
	return p.kimiBase
}

func (p *KimiProvider) MatchesPath(path string) bool {
	// Kimi is routed by model name, not by path.
	// It never wins path-based routing.
	return false
}

func (p *KimiProvider) NormalizePath(path string) string {
	return path
}

func (p *KimiProvider) DetectsSSE(path string, contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

// kimiModels lists model names that should be routed to the Kimi provider.
// Pi's built-in kimi-coding provider exposes both short aliases and provider-native IDs.
// isKimiModel returns true if the given model name should be routed to Kimi.
func isKimiModel(model string) bool {
	_, ok := modelForProvider(AccountTypeKimi, model)
	return ok
}

func parseKimiResponseRateLimits(headers http.Header) (UsageSnapshot, bool) {
	if headers == nil {
		return UsageSnapshot{}, false
	}

	snap := UsageSnapshot{
		RetrievedAt: time.Now(),
		Source:      "headers",
	}
	hasPrimary := false
	hasSecondary := false

	if pct, ok := parseRateLimitPercent(headers.Get("x-ratelimit-requests-utilization")); ok {
		snap.PrimaryUsedPercent = pct
		snap.PrimaryUsed = pct
		hasPrimary = true
	} else if pct, ok := parseRateLimitUsageFromRemainingLimit(headers, "x-ratelimit-remaining-requests", "x-ratelimit-limit-requests"); ok {
		snap.PrimaryUsedPercent = pct
		snap.PrimaryUsed = pct
		hasPrimary = true
	} else if pct, ok := parseRateLimitUsageFromRemainingLimit(headers, "x-ratelimit-requests-remaining", "x-ratelimit-requests-limit"); ok {
		snap.PrimaryUsedPercent = pct
		snap.PrimaryUsed = pct
		hasPrimary = true
	} else if pct, ok := parseRateLimitUsageFromRemainingLimit(headers, "x-ratelimit-remaining", "x-ratelimit-limit"); ok {
		snap.PrimaryUsedPercent = pct
		snap.PrimaryUsed = pct
		hasPrimary = true
	}

	if pct, ok := parseRateLimitPercent(headers.Get("x-ratelimit-tokens-utilization")); ok {
		snap.SecondaryUsedPercent = pct
		snap.SecondaryUsed = pct
		hasSecondary = true
	} else if pct, ok := parseRateLimitUsageFromRemainingLimit(headers, "x-ratelimit-remaining-tokens", "x-ratelimit-limit-tokens"); ok {
		snap.SecondaryUsedPercent = pct
		snap.SecondaryUsed = pct
		hasSecondary = true
	}

	if !hasPrimary && !hasSecondary {
		return UsageSnapshot{}, false
	}

	requestsReset, ok := parseRateLimitReset(headers.Get("x-ratelimit-reset-requests"))
	if !ok {
		requestsReset, ok = parseRateLimitReset(headers.Get("x-ratelimit-requests-reset"))
	}
	if ok {
		snap.PrimaryResetAt = requestsReset
	}

	tokensReset, ok := parseRateLimitReset(headers.Get("x-ratelimit-reset-tokens"))
	if !ok {
		tokensReset, ok = parseRateLimitReset(headers.Get("x-ratelimit-tokens-reset"))
	}
	if ok {
		snap.SecondaryResetAt = tokensReset
	}

	return snap, true
}
