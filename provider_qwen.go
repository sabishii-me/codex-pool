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

// QwenProvider handles Qwen (Alibaba DashScope Coding Plan) accounts through
// its Anthropic-compatible API.
type QwenProvider struct {
	qwenBase *url.URL
}

// NewQwenProvider creates a new Qwen provider.
func NewQwenProvider(qwenBase *url.URL) *QwenProvider {
	return &QwenProvider{
		qwenBase: qwenBase,
	}
}

func (p *QwenProvider) Type() AccountType {
	return AccountTypeQwen
}

type QwenAuthJSON struct {
	APIKey string `json:"api_key"`
}

func (p *QwenProvider) LoadAccount(name, path string, data []byte) (*ProviderConnection, error) {
	var qj QwenAuthJSON
	if err := json.Unmarshal(data, &qj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if qj.APIKey == "" {
		return nil, nil
	}

	acc := &ProviderConnection{
		Type:        AccountTypeQwen,
		ID:          strings.TrimSuffix(name, filepath.Ext(name)),
		File:        path,
		AccessToken: qj.APIKey,
		PlanType:    "qwen",
	}
	return acc, nil
}

func (p *QwenProvider) SetAuthHeaders(req *http.Request, acc *ProviderConnection) {
	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
}

func (p *QwenProvider) RefreshToken(ctx context.Context, acc *ProviderConnection, transport http.RoundTripper) error {
	return nil
}

func (p *QwenProvider) ParseUsage(obj map[string]any) *RequestUsage {
	if usage := parseAnthropicUsage(obj); usage != nil {
		return usage
	}
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

func (p *QwenProvider) ParseUsageHeaders(acc *ProviderConnection, headers http.Header) {
	// Qwen's Anthropic-compatible endpoint does not currently expose quota headers.
}

func (p *QwenProvider) UpstreamURL(path string) *url.URL {
	return p.qwenBase
}

func (p *QwenProvider) MatchesPath(path string) bool {
	// Qwen is model-routed.
	return false
}

func (p *QwenProvider) NormalizePath(path string) string {
	return path
}

func (p *QwenProvider) DetectsSSE(path string, contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

func isQwenModel(model string) bool {
	_, ok := modelForProvider(AccountTypeQwen, model)
	return ok
}

func qwenCanonicalModel(model string) string {
	if found, ok := modelForProvider(AccountTypeQwen, model); ok {
		return found.ID
	}
	return model
}
