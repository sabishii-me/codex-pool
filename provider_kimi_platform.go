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

// KimiPlatformProvider handles a pay-as-you-go Kimi Open Platform API key,
// which is a distinct product from the Kimi Coding Plan. It talks to Kimi's
// Anthropic-compatible endpoint. Requests are routed by either a real bare
// Open Platform model ID (for example, "kimi-k3") or an explicit
// "kimi-platform/" prefix (for example, "kimi-platform/kimi-k3").
type KimiPlatformProvider struct {
	kimiPlatformBase *url.URL
}

// NewKimiPlatformProvider creates a new Kimi Platform provider.
func NewKimiPlatformProvider(kimiPlatformBase *url.URL) *KimiPlatformProvider {
	return &KimiPlatformProvider{
		kimiPlatformBase: kimiPlatformBase,
	}
}

func (p *KimiPlatformProvider) Type() AccountType {
	return AccountTypeKimiPlatform
}

// KimiPlatformAuthJSON is the format for Kimi Platform auth files.
type KimiPlatformAuthJSON struct {
	APIKey string `json:"api_key"`
}

func (p *KimiPlatformProvider) LoadAccount(name, path string, data []byte) (*Account, error) {
	var kj KimiPlatformAuthJSON
	if err := json.Unmarshal(data, &kj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if kj.APIKey == "" {
		return nil, nil
	}

	acc := &Account{
		Type:        AccountTypeKimiPlatform,
		ID:          strings.TrimSuffix(name, filepath.Ext(name)),
		File:        path,
		AccessToken: kj.APIKey,
		PlanType:    "kimi-platform",
	}
	return acc, nil
}

func (p *KimiPlatformProvider) SetAuthHeaders(req *http.Request, acc *Account) {
	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
}

func (p *KimiPlatformProvider) RefreshToken(ctx context.Context, acc *Account, transport http.RoundTripper) error {
	// API keys don't need refresh
	return nil
}

func (p *KimiPlatformProvider) ParseUsage(obj map[string]any) *RequestUsage {
	if usage := parseAnthropicUsage(obj); usage != nil {
		return usage
	}
	// Kimi Platform proxies Anthropic/OpenAI-style responses, so parse both formats.

	// OpenAI-style usage object
	if usageMap, ok := obj["usage"].(map[string]any); ok {
		ru := &RequestUsage{Timestamp: time.Now()}
		ru.InputTokens = readInt64(usageMap, "prompt_tokens")
		if ru.InputTokens == 0 {
			ru.InputTokens = readInt64(usageMap, "input_tokens")
		}
		ru.OutputTokens = readInt64(usageMap, "completion_tokens")
		if ru.OutputTokens == 0 {
			ru.OutputTokens = readInt64(usageMap, "output_tokens")
		}
		ru.CachedInputTokens = readInt64(usageMap, "cached_tokens")
		if ru.InputTokens == 0 && ru.OutputTokens == 0 {
			return nil
		}
		ru.BillableTokens = ru.InputTokens + ru.OutputTokens
		if m, ok := obj["model"].(string); ok && m != "" {
			ru.Model = m
		}
		return ru
	}

	// Anthropic-style: message_start / message_delta
	eventType, _ := obj["type"].(string)
	if eventType == "message_delta" {
		usageMap, ok := obj["usage"].(map[string]any)
		if !ok {
			return nil
		}
		ru := &RequestUsage{Timestamp: time.Now()}
		ru.OutputTokens = readInt64(usageMap, "output_tokens")
		if ru.OutputTokens == 0 {
			return nil
		}
		ru.BillableTokens = ru.OutputTokens
		return ru
	}
	if eventType == "message_start" {
		msg, ok := obj["message"].(map[string]any)
		if !ok {
			return nil
		}
		usageMap, ok := msg["usage"].(map[string]any)
		if !ok {
			return nil
		}
		ru := &RequestUsage{Timestamp: time.Now()}
		ru.InputTokens = readInt64(usageMap, "input_tokens")
		ru.CachedInputTokens = readInt64(usageMap, "cache_read_input_tokens")
		if ru.InputTokens == 0 {
			return nil
		}
		if model, ok := msg["model"].(string); ok {
			ru.Model = model
		}
		ru.BillableTokens = clampNonNegative(ru.InputTokens - ru.CachedInputTokens)
		return ru
	}

	return nil
}

func (p *KimiPlatformProvider) ParseUsageHeaders(acc *Account, headers http.Header) {
	// The Open Platform's Anthropic-compatible endpoint does not currently
	// expose the coding-plan-style quota headers, so there is nothing to parse.
}

func (p *KimiPlatformProvider) UpstreamURL(path string) *url.URL {
	return p.kimiPlatformBase
}

func (p *KimiPlatformProvider) MatchesPath(path string) bool {
	// Kimi Platform is routed by model prefix, not by path.
	return false
}

func (p *KimiPlatformProvider) NormalizePath(path string) string {
	return path
}

func (p *KimiPlatformProvider) DetectsSSE(path string, contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

const kimiPlatformModelPrefix = "kimi-platform/"

// isKimiPlatformModel reports whether a request model should route to a
// pooled Kimi Platform account. It matches:
//
//	(1) an explicit "kimi-platform/" prefix (e.g. "kimi-platform/kimi-k3")
//	(2) bare model IDs that correspond to known Open Platform models
//	    (e.g. "kimi-k3", "kimi-k2.7-code") that are not claimed by the
//	    Coding Plan's (AccountTypeKimi) catalog.
//
// This prevents the product/model conflation where "kimi-for-coding" (a
// Coding Plan model) would incorrectly route to a platform account.
func isKimiPlatformModel(model string) bool {
	m := strings.TrimSpace(model)
	if strings.HasPrefix(m, kimiPlatformModelPrefix) {
		return true
	}
	// Also match bare platform model IDs (those not in the Coding Plan catalog).
	// modelForProvider(AccountTypeKimiPlatform, m) will match real platform
	// model IDs like kimi-k3, kimi-k2.7-code, etc. that are listed in the
	// poolModels catalog under AccountTypeKimiPlatform with no prefix.
	_, ok := modelForProvider(AccountTypeKimiPlatform, m)
	return ok
}

// kimiPlatformCanonicalModel returns the upstream model ID.
// For prefixed models (e.g. "kimi-platform/kimi-k3"), it strips the prefix.
// For bare platform models (e.g. "kimi-k3"), it returns the model as-is.
func kimiPlatformCanonicalModel(model string) string {
	m := strings.TrimSpace(model)
	if strings.HasPrefix(m, kimiPlatformModelPrefix) {
		return strings.TrimPrefix(m, kimiPlatformModelPrefix)
	}
	// Bare platform model: look up the canonical ID from the catalog.
	if found, ok := modelForProvider(AccountTypeKimiPlatform, m); ok {
		return found.ID
	}
	return m
}
