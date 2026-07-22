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

// NvidiaProvider handles NVIDIA NIM accounts. Unlike the other "extra"
// providers, NVIDIA has no Anthropic-compatible endpoint - it only speaks
// standard OpenAI Chat Completions, so its target format is FormatOpenAI
// (see format_translate.go's providerTargetFormat) and its usage parsing
// reads OpenAI's usage shape, not Anthropic's.
//
// Like OpenRouter, NVIDIA is an aggregator with vendor-prefixed model ids
// (e.g. "meta/llama-3.3-70b-instruct") - no fixed catalog, requests are
// routed here via an explicit "nvidia/" prefix (see isNvidiaModel below).
type NvidiaProvider struct {
	nvidiaBase *url.URL
}

// NewNvidiaProvider creates a new NVIDIA provider.
func NewNvidiaProvider(nvidiaBase *url.URL) *NvidiaProvider {
	return &NvidiaProvider{
		nvidiaBase: nvidiaBase,
	}
}

func (p *NvidiaProvider) Type() AccountType {
	return AccountTypeNvidia
}

type NvidiaAuthJSON struct {
	APIKey string `json:"api_key"`
}

func (p *NvidiaProvider) LoadAccount(name, path string, data []byte) (*ProviderConnection, error) {
	var nj NvidiaAuthJSON
	if err := json.Unmarshal(data, &nj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if nj.APIKey == "" {
		return nil, nil
	}

	acc := &ProviderConnection{
		Type:        AccountTypeNvidia,
		ID:          strings.TrimSuffix(name, filepath.Ext(name)),
		File:        path,
		AccessToken: nj.APIKey,
		PlanType:    "nvidia",
	}
	return acc, nil
}

func (p *NvidiaProvider) SetAuthHeaders(req *http.Request, acc *ProviderConnection) {
	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
}

func (p *NvidiaProvider) RefreshToken(ctx context.Context, acc *ProviderConnection, transport http.RoundTripper) error {
	return nil
}

// ParseUsage reads OpenAI Chat Completions' usage shape: a top-level "usage"
// object with prompt_tokens/completion_tokens, present on the non-streaming
// response and (when requested) the final SSE chunk.
func (p *NvidiaProvider) ParseUsage(obj map[string]any) *RequestUsage {
	return openAIChatEngine.ParseUsage(obj)
}

func (p *NvidiaProvider) ParseUsageHeaders(acc *ProviderConnection, headers http.Header) {
	// NVIDIA's OpenAI-compatible endpoint does not currently expose quota headers.
}

func (p *NvidiaProvider) UpstreamURL(path string) *url.URL {
	return p.nvidiaBase
}

func (p *NvidiaProvider) MatchesPath(path string) bool {
	// NVIDIA is model-routed.
	return false
}

func (p *NvidiaProvider) NormalizePath(path string) string {
	return path
}

func (p *NvidiaProvider) DetectsSSE(path string, contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

const nvidiaModelPrefix = "nvidia/"

// isNvidiaModel reports whether a request model should route to a pooled
// NVIDIA account - identified by an explicit "nvidia/" prefix rather than a
// fixed catalog, since NVIDIA NIM routes to any vendor-prefixed hosted model.
func isNvidiaModel(model string) bool {
	return strings.HasPrefix(strings.TrimSpace(model), nvidiaModelPrefix)
}

// nvidiaCanonicalModel strips the "nvidia/" prefix, leaving the vendor-prefixed
// model id (e.g. "meta/llama-3.3-70b-instruct") to forward upstream.
func nvidiaCanonicalModel(model string) string {
	return strings.TrimPrefix(strings.TrimSpace(model), nvidiaModelPrefix)
}
