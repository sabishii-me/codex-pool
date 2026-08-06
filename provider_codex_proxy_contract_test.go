package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexProxyStreamingCanonicalUsageAndResponseIntegrity(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	completed := `{"type":"response.completed","response":{"id":"resp_contract","model":"gpt-5.5","status":"completed","output":[],"usage":{"input_tokens":120,"output_tokens":40,"input_tokens_details":{"cached_tokens":30},"output_tokens_details":{"reasoning_tokens":7}}}}`
	sseBody := []byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_contract\",\"model\":\"gpt-5.5\",\"status\":\"in_progress\"}}\n\n" +
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"unchanged\"}\n\n" +
		"event: response.completed\ndata: " + completed + "\n\n")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sseBody)
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	codex := NewCodexProvider(base, base, base)
	registry := NewProviderRegistry(codex, NewGeminiProvider(base, base))
	account := &Account{Type: AccountTypeCodex, ID: "codex_stream", AccessToken: "contract-key", PlanType: "plus"}
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.db.Close()
	handler := &proxyHandler{cfg: &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024}, transport: http.DefaultTransport, pool: newProviderPool([]*Account{account}), registry: registry, analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil)}
	requestBody := []byte(`{"model":"gpt-5.5","input":"hello","stream":true}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	requestID := "codex-stream-contract"
	handler.proxyRequest(response, request, requestID)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), sseBody) {
		t.Fatalf("response status=%d body=%q", response.Code, response.Body.Bytes())
	}
	assertCanonicalCustomUsage(t, analytics, account, requestID, "contract-user", 120, 30, 0, 40, 7, 130)
}

func TestCodexToAnthropicStreamFinalizesUnterminatedCompletedEvent(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	completed := `{"type":"response.completed","response":{"id":"resp_terminal","model":"gpt-5.6-sol","status":"completed","output":[],"usage":{"input_tokens":7,"output_tokens":5}}}`
	// The response.completed event intentionally has no trailing blank line.
	sseBody := []byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_terminal\",\"model\":\"gpt-5.6-sol\",\"status\":\"in_progress\"}}\n\n" +
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n" +
		"event: response.completed\ndata: " + completed)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sseBody)
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	registry := NewProviderRegistry(NewCodexProvider(base, base, base), NewGeminiProvider(base, base))
	connection := &ProviderConnection{Type: AccountTypeCodex, ID: "codex_terminal", AccessToken: "contract-key", PlanType: "plus"}
	handler := &proxyHandler{
		cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
		transport: http.DefaultTransport, pool: newProviderPool([]*ProviderConnection{connection}), registry: registry,
		metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"gpt-5.6-sol","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "codex-terminal-contract")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if count := strings.Count(response.Body.String(), "event: message_stop"); count != 1 {
		t.Fatalf("message_stop count=%d body=%s", count, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "event: error") {
		t.Fatalf("successful terminal event produced error: %s", response.Body.String())
	}
}

func TestCodexProxyCanonicalizesSSEContentTypeForImageTool(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	sseBody := []byte("event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"status\":\"in_progress\"}}\n\n" +
		"event: response.image_generation_call.completed\ndata: {\"type\":\"response.image_generation_call.completed\",\"item_id\":\"ig_1\",\"result\":\"AAAA\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_image\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":7,\"output_tokens\":5}}}\n\n")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// This matches the observed Codex image-tool response: valid SSE bytes
		// with a generic media type.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(sseBody)
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	connection := &ProviderConnection{Type: AccountTypeCodex, ID: "codex_image", AccessToken: "contract-key", PlanType: "plus"}
	handler := &proxyHandler{
		cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
		transport: http.DefaultTransport, pool: newProviderPool([]*ProviderConnection{connection}),
		registry: NewProviderRegistry(NewCodexProvider(base, base, base), NewGeminiProvider(base, base)),
		metrics:  newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
	}
	body := `{"model":"gpt-5.6-sol","instructions":"use image tool","input":"draw a square","tools":[{"type":"image_generation"}],"tool_choice":{"type":"image_generation"},"stream":true}`
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "codex-image-tool-contract")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type=%q, want text/event-stream", got)
	}
	if !bytes.Equal(response.Body.Bytes(), sseBody) {
		t.Fatal("image tool event stream changed")
	}
}

func TestCodexProxyRejectsLargeNativeResponsesBeforeUpstream(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	var forwarded []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		forwarded, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_large","object":"response","model":"gpt-5.5","status":"completed","output":[],"usage":{"input_tokens":120,"output_tokens":40,"input_tokens_details":{"cached_tokens":30},"output_tokens_details":{"reasoning_tokens":7}}}`))
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	codex := NewCodexProvider(base, base, base)
	registry := NewProviderRegistry(codex, NewGeminiProvider(base, base))
	account := &Account{Type: AccountTypeCodex, ID: "codex_large", AccessToken: "contract-key", PlanType: "plus"}
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.db.Close()
	handler := &proxyHandler{cfg: &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024}, transport: http.DefaultTransport, pool: newProviderPool([]*Account{account}), registry: registry, analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil)}
	padding := strings.Repeat("x", streamedModelRoutePeekBytes+1024)
	requestBody := []byte(`{"model":"gpt-5.5","input":` + mustJSONContractString(t, padding) + `,"stream":false}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	request.ContentLength = int64(len(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "codex-large-contract")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("response status=%d body=%s", response.Code, response.Body.String())
	}
	if forwarded != nil {
		t.Fatalf("large native Responses body reached upstream: %d bytes", len(forwarded))
	}
}
