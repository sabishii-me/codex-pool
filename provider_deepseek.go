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

func (p *DeepSeekProvider) LoadAccount(name, path string, data []byte) (*ProviderConnection, error) {
	var dj DeepSeekAuthJSON
	if err := json.Unmarshal(data, &dj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if dj.APIKey == "" {
		return nil, nil
	}

	acc := &ProviderConnection{
		Type:        AccountTypeDeepSeek,
		ID:          strings.TrimSuffix(name, filepath.Ext(name)),
		File:        path,
		AccessToken: dj.APIKey,
		PlanType:    "deepseek",
	}
	return acc, nil
}

func (p *DeepSeekProvider) SetAuthHeaders(req *http.Request, acc *ProviderConnection) {
	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
}

func (p *DeepSeekProvider) RefreshToken(ctx context.Context, acc *ProviderConnection, transport http.RoundTripper) error {
	return nil
}

func (p *DeepSeekProvider) ParseUsage(obj map[string]any) *RequestUsage {
	return anthropicMessagesEngine.ParseUsage(obj)
}

func (p *DeepSeekProvider) ParseUsageHeaders(acc *ProviderConnection, headers http.Header) {
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
	return eventStreamDetector.Detect(path, contentType)
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
