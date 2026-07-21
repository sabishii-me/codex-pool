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

// DeepSeekProvider handles DeepSeek accounts through DeepSeek's Anthropic-compatible API.
type DeepSeekProvider struct {
	deepseekBase *url.URL
}

// NewDeepSeekProvider creates a new DeepSeek provider.
func NewDeepSeekProvider(deepseekBase *url.URL) *DeepSeekProvider {
	return &DeepSeekProvider{
		deepseekBase: deepseekBase,
	}
}

func (p *DeepSeekProvider) Type() AccountType {
	return AccountTypeDeepSeek
}

type DeepSeekAuthJSON struct {
	APIKey string `json:"api_key"`
}

func (p *DeepSeekProvider) LoadAccount(name, path string, data []byte) (*Account, error) {
	var dj DeepSeekAuthJSON
	if err := json.Unmarshal(data, &dj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if dj.APIKey == "" {
		return nil, nil
	}

	acc := &Account{
		Type:        AccountTypeDeepSeek,
		ID:          strings.TrimSuffix(name, filepath.Ext(name)),
		File:        path,
		AccessToken: dj.APIKey,
		PlanType:    "deepseek",
	}
	return acc, nil
}

func (p *DeepSeekProvider) SetAuthHeaders(req *http.Request, acc *Account) {
	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
}

func (p *DeepSeekProvider) RefreshToken(ctx context.Context, acc *Account, transport http.RoundTripper) error {
	return nil
}

func (p *DeepSeekProvider) ParseUsage(obj map[string]any) *RequestUsage {
	eventType, _ := obj["type"].(string)

	if eventType == "message_delta" {
		usageMap, ok := obj["usage"].(map[string]any)
		if !ok {
			return nil
		}
		ru := &RequestUsage{Timestamp: time.Now()}
		ru.InputTokens = readInt64(usageMap, "input_tokens")
		ru.CachedInputTokens = readInt64(usageMap, "cache_read_input_tokens")
		ru.CacheCreationTokens = readInt64(usageMap, "cache_creation_input_tokens")
		ru.OutputTokens = readInt64(usageMap, "output_tokens")
		if ru.InputTokens == 0 && ru.OutputTokens == 0 {
			return nil
		}
		ru.BillableTokens = clampNonNegative(ru.InputTokens - ru.CachedInputTokens - ru.CacheCreationTokens + ru.OutputTokens)
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
		ru.CacheCreationTokens = readInt64(usageMap, "cache_creation_input_tokens")
		if ru.InputTokens == 0 {
			return nil
		}
		if model, ok := msg["model"].(string); ok {
			ru.Model = model
		}
		ru.BillableTokens = clampNonNegative(ru.InputTokens - ru.CachedInputTokens - ru.CacheCreationTokens)
		return ru
	}

	return nil
}

func (p *DeepSeekProvider) ParseUsageHeaders(acc *Account, headers http.Header) {
	// DeepSeek's Anthropic-compatible endpoint does not currently expose quota headers.
}

func (p *DeepSeekProvider) UpstreamURL(path string) *url.URL {
	return p.deepseekBase
}

func (p *DeepSeekProvider) MatchesPath(path string) bool {
	// DeepSeek is model-routed.
	return false
}

func (p *DeepSeekProvider) NormalizePath(path string) string {
	return path
}

func (p *DeepSeekProvider) DetectsSSE(path string, contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

func isDeepSeekModel(model string) bool {
	_, ok := modelForProvider(AccountTypeDeepSeek, model)
	return ok
}

func deepseekCanonicalModel(model string) string {
	if found, ok := modelForProvider(AccountTypeDeepSeek, model); ok {
		return found.ID
	}
	return model
}
