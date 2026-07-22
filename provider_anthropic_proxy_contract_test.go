package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type anthropicProxyContractCase struct {
	ProviderType AccountType
	Model        string
}

func TestAnthropicCompatibleProvidersProxyNonStreamingExactlyOnce(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	responseObject := map[string]any{
		"id": "msg_contract", "type": "message", "role": "assistant", "model": "contract-upstream-model",
		"content": []any{map[string]any{"type": "text", "text": "unchanged"}},
		"usage": map[string]any{
			"input_tokens": float64(120), "cache_read_input_tokens": float64(30),
			"cache_creation_input_tokens": float64(20), "output_tokens": float64(40),
			"output_tokens_details": map[string]any{"reasoning_tokens": float64(7)},
		},
	}
	responseBody, err := json.Marshal(responseObject)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responseBody)
	}))
	defer upstream.Close()
	base, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	cases := []anthropicProxyContractCase{
		{AccountTypeClaude, "claude-contract-model"},
		{AccountTypeKimi, "kimi-for-coding"},
		{AccountTypeKimiPlatform, "kimi-k3"},
		{AccountTypeMinimax, "MiniMax-M3"},
		{AccountTypeZAI, "glm-5.2"},
		{AccountTypeXiaomi, "mimo-v2.5-pro"},
		{AccountTypeDeepSeek, "deepseek-v4-pro"},
		{AccountTypeQwen, "qwen3.6-plus"},
		{AccountTypeOpenRouter, "openrouter/anthropic/claude-haiku-4.5"},
	}
	for _, test := range cases {
		test := test
		t.Run(string(test.ProviderType), func(t *testing.T) {
			registry := anthropicContractRegistry(base)
			account := &Account{Type: test.ProviderType, ID: string(test.ProviderType) + "_contract", AccessToken: "contract-key", PlanType: string(test.ProviderType)}
			analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer analytics.db.Close()
			handler := &proxyHandler{
				cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
				transport: http.DefaultTransport, pool: newProviderPool([]*Account{account}, false), registry: registry,
				analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5),
			}
			requestBody := []byte(`{"model":"` + test.Model + `","messages":[{"role":"user","content":"hi"}],"stream":false}`)
			request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(requestBody))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
			response := httptest.NewRecorder()
			handler.proxyRequest(response, request, "contract-"+string(test.ProviderType))

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if !bytes.Equal(response.Body.Bytes(), responseBody) {
				t.Fatalf("response bytes changed\n got: %s\nwant: %s", response.Body.Bytes(), responseBody)
			}
			account.mu.Lock()
			totals := account.Totals
			account.mu.Unlock()
			if totals.RequestCount != 1 || totals.TotalInputTokens != 120 || totals.TotalCachedTokens != 30 || totals.TotalOutputTokens != 40 || totals.TotalReasoningTokens != 7 || totals.TotalBillableTokens != 110 {
				t.Fatalf("unexpected totals: %+v", totals)
			}
			assertCanonicalContractUsage(t, analytics, account, "contract-"+string(test.ProviderType))
		})
	}
}

func TestAnthropicCompatibleProvidersProxyStreamingExactlyOnce(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	start := `{"type":"message_start","message":{"id":"msg_contract","type":"message","role":"assistant","model":"contract-upstream-model","content":[],"usage":{"input_tokens":120,"cache_read_input_tokens":30,"cache_creation_input_tokens":20}}}`
	delta := `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":40,"reasoning_tokens":7}}`
	sseBody := []byte("event: message_start\ndata: " + start + "\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"unchanged\"}}\n\n" +
		"event: message_delta\ndata: " + delta + "\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(sseBody)
	}))
	defer upstream.Close()
	base, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	cases := []anthropicProxyContractCase{
		{AccountTypeClaude, "claude-contract-model"},
		{AccountTypeKimi, "kimi-for-coding"}, {AccountTypeKimiPlatform, "kimi-k3"},
		{AccountTypeMinimax, "MiniMax-M3"}, {AccountTypeZAI, "glm-5.2"},
		{AccountTypeXiaomi, "mimo-v2.5-pro"}, {AccountTypeDeepSeek, "deepseek-v4-pro"},
		{AccountTypeQwen, "qwen3.6-plus"}, {AccountTypeOpenRouter, "openrouter/anthropic/claude-haiku-4.5"},
	}
	for _, test := range cases {
		test := test
		t.Run(string(test.ProviderType), func(t *testing.T) {
			registry := anthropicContractRegistry(base)
			account := &Account{Type: test.ProviderType, ID: string(test.ProviderType) + "_stream_contract", AccessToken: "contract-key", PlanType: string(test.ProviderType)}
			analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer analytics.db.Close()
			handler := &proxyHandler{
				cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
				transport: http.DefaultTransport, pool: newProviderPool([]*Account{account}, false), registry: registry,
				analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5),
			}
			requestBody := []byte(`{"model":"` + test.Model + `","messages":[{"role":"user","content":"hi"}],"stream":true}`)
			request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(requestBody))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "text/event-stream")
			request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
			response := httptest.NewRecorder()
			handler.proxyRequest(response, request, "stream-contract-"+string(test.ProviderType))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if !bytes.Equal(response.Body.Bytes(), sseBody) {
				t.Fatalf("SSE response bytes changed\n got: %q\nwant: %q", response.Body.Bytes(), sseBody)
			}
			account.mu.Lock()
			totals := account.Totals
			account.mu.Unlock()
			if totals.RequestCount != 1 || totals.TotalInputTokens != 120 || totals.TotalCachedTokens != 30 || totals.TotalOutputTokens != 40 || totals.TotalReasoningTokens != 7 || totals.TotalBillableTokens != 110 {
				t.Fatalf("unexpected totals: %+v", totals)
			}
			assertCanonicalContractUsage(t, analytics, account, "stream-contract-"+string(test.ProviderType))
		})
	}
}

func TestAnthropicCompatibleProvidersProxyLargeBodyRouteAndUsage(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	responseBody := []byte(`{"id":"msg_large","type":"message","role":"assistant","model":"contract-upstream-model","content":[{"type":"text","text":"unchanged"}],"usage":{"input_tokens":120,"cache_read_input_tokens":30,"cache_creation_input_tokens":20,"output_tokens":40,"reasoning_tokens":7}}`)
	type largeBodyCase struct {
		ProviderType AccountType
		Model        string
		Canonical    string
	}
	cases := []largeBodyCase{
		{AccountTypeClaude, "claude-contract-model", "claude-contract-model"},
		{AccountTypeKimi, "kimi-for-coding", "kimi-for-coding"},
		{AccountTypeKimiPlatform, "kimi-platform/kimi-k3", "kimi-k3"},
		{AccountTypeMinimax, "minimax", "MiniMax-M3"},
		{AccountTypeZAI, "glm-5.2", "glm-5.2"},
		{AccountTypeXiaomi, "mimo-v2.5-pro", "mimo-v2.5-pro"},
		{AccountTypeDeepSeek, "deepseek", "deepseek-v4-pro"},
		{AccountTypeQwen, "qwen-coder", "qwen3.6-plus"},
		{AccountTypeOpenRouter, "openrouter/anthropic/claude-haiku-4.5", "anthropic/claude-haiku-4.5"},
	}
	for _, test := range cases {
		test := test
		t.Run(string(test.ProviderType), func(t *testing.T) {
			var upstreamBody []byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				upstreamBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(responseBody)
			}))
			defer upstream.Close()
			base, _ := url.Parse(upstream.URL)
			registry := anthropicContractRegistry(base)
			account := &Account{Type: test.ProviderType, ID: string(test.ProviderType) + "_large_contract", AccessToken: "contract-key", PlanType: string(test.ProviderType)}
			analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer analytics.db.Close()
			handler := &proxyHandler{
				cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024},
				transport: http.DefaultTransport, pool: newProviderPool([]*Account{account}, false), registry: registry,
				analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
			}
			padding := strings.Repeat("x", streamedModelRoutePeekBytes+1024)
			requestBody := []byte(`{"model":"` + test.Model + `","messages":[{"role":"user","content":"` + padding + `"}],"stream":false}`)
			request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(requestBody))
			request.ContentLength = int64(len(requestBody))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
			response := httptest.NewRecorder()
			handler.proxyRequest(response, request, "large-contract-"+string(test.ProviderType))
			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), responseBody) {
				t.Fatalf("response status=%d body=%s", response.Code, response.Body.String())
			}
			var forwarded map[string]any
			if err := json.Unmarshal(upstreamBody, &forwarded); err != nil {
				t.Fatalf("upstream body is invalid JSON: %v", err)
			}
			if forwarded["model"] != test.Canonical {
				t.Fatalf("upstream model = %v, want %s", forwarded["model"], test.Canonical)
			}
			messages := forwarded["messages"].([]any)
			content := messages[0].(map[string]any)["content"].(string)
			if content != padding {
				t.Fatalf("large request content changed: got %d bytes, want %d", len(content), len(padding))
			}
			account.mu.Lock()
			totals := account.Totals
			account.mu.Unlock()
			if totals.RequestCount != 1 || totals.TotalBillableTokens != 110 || totals.TotalReasoningTokens != 7 {
				t.Fatalf("unexpected totals: %+v", totals)
			}
			assertCanonicalContractUsage(t, analytics, account, "large-contract-"+string(test.ProviderType))
		})
	}
}

func assertCanonicalContractUsage(t *testing.T, analytics *AnalyticsStore, account *Account, requestID string) {
	t.Helper()
	var count, input, cacheRead, cacheWrite, output, reasoning, billable int64
	var persistedRequestID, userID, providerID, connectionID string
	err := analytics.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(input_tokens),0),
		COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_write_tokens),0),
		COALESCE(SUM(output_tokens),0), COALESCE(SUM(reasoning_tokens),0),
		COALESCE(SUM(billable_tokens),0), COALESCE(MAX(request_id),''),
		COALESCE(MAX(user_id),''), COALESCE(MAX(provider_id),''), COALESCE(MAX(connection_id),'')
		FROM usage_events WHERE connection_id = ?`, account.ID).Scan(
		&count, &input, &cacheRead, &cacheWrite, &output, &reasoning, &billable,
		&persistedRequestID, &userID, &providerID, &connectionID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || input != 120 || cacheRead != 30 || cacheWrite != 20 || output != 40 || reasoning != 7 || billable != 110 || persistedRequestID != requestID || userID != "contract-user" || providerID != string(account.Type) || connectionID != account.ID {
		t.Fatalf("canonical usage count=%d input=%d cache-read=%d cache-write=%d output=%d reasoning=%d billable=%d request=%q user=%q provider=%q connection=%q",
			count, input, cacheRead, cacheWrite, output, reasoning, billable,
			persistedRequestID, userID, providerID, connectionID)
	}
}

func anthropicContractRegistry(base *url.URL) *ProviderRegistry {
	codex := NewCodexProvider(base, base, base)
	claude := NewClaudeProvider(base)
	gemini := NewGeminiProvider(base, base)
	return NewProviderRegistry(codex, claude, gemini,
		NewKimiProvider(base), NewKimiPlatformProvider(base), NewMinimaxProvider(base), NewZAIProvider(base),
		NewXiaomiProvider(base), NewDeepSeekProvider(base), NewQwenProvider(base), NewOpenRouterProvider(base),
	)
}
