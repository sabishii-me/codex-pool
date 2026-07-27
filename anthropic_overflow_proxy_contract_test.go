package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newCodexAnthropicContractHandler(t *testing.T, upstream http.HandlerFunc) (*proxyHandler, func()) {
	t.Helper()
	server := httptest.NewServer(upstream)
	base, _ := url.Parse(server.URL)
	connection := &ProviderConnection{Type: AccountTypeCodex, ID: "codex_anthropic", AccessToken: "contract-key", PlanType: "plus"}
	handler := &proxyHandler{
		cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
		transport: http.DefaultTransport, pool: newProviderPool([]*ProviderConnection{connection}),
		registry: NewProviderRegistry(NewCodexProvider(base, base, base), NewClaudeProvider(base), NewGeminiProvider(base, base)),
		metrics:  newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
	}
	return handler, server.Close
}

func TestCodexAnthropicBufferedOverflowReturnsHTTPError(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	handler, closeServer := newCodexAnthropicContractHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(contextOverflowFixture))
	})
	defer closeServer()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"gpt-5.6-sol","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"oversized"}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "buffered-overflow-contract")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"type":"invalid_request_error"`) {
		t.Fatalf("body=%s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"role":"assistant"`) || strings.Contains(response.Body.String(), "[Error:") {
		t.Fatalf("failure became assistant message: %s", response.Body.String())
	}
}

func TestCodexAnthropicStreamingOverflowEmitsErrorEvent(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	handler, closeServer := newCodexAnthropicContractHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(contextOverflowFixture))
	})
	defer closeServer()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"gpt-5.6-sol","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"oversized"}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "stream-overflow-contract")
	wire := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d wire=%s", response.Code, wire)
	}
	if !strings.Contains(wire, "event: error") || !strings.Contains(wire, `"type":"invalid_request_error"`) {
		t.Fatalf("wire=%s", wire)
	}
	if strings.Contains(wire, "content_block_delta") || strings.Contains(wire, "message_stop") || strings.Contains(wire, "[Error:") {
		t.Fatalf("failure emitted success protocol:\n%s", wire)
	}
}
