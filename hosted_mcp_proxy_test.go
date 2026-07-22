package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCodexProxyFiltersHostedMCPRequestAndJSONResponse(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "hosted-mcp-test")
	base, _ := url.Parse("https://chatgpt.com/backend-api")
	connection := &ProviderConnection{Type: AccountTypeCodex, ID: "codex", AccessToken: "token", AccountID: "upstream", PlanType: "pro"}
	upstreamCalled := false
	handler := &proxyHandler{
		cfg: &config{maxAttempts: 1, maxInMemoryBodyBytes: 4096},
		transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			upstreamCalled = true
			body, _ := io.ReadAll(request.Body)
			text := string(body)
			for _, forbidden := range []string{`"type":"mcp"`, "private.example", "approval-secret"} {
				if strings.Contains(text, forbidden) {
					t.Errorf("upstream request leaked %q: %s", forbidden, text)
				}
			}
			for _, required := range []string{"web_search", "local_mcp_tool"} {
				if !strings.Contains(text, required) {
					t.Errorf("upstream request removed %q: %s", required, text)
				}
			}
			response := `{"id":"resp","output":[{"type":"mcp_call","output":"TOP-SECRET"},{"type":"web_search_call"},{"type":"message","content":[{"type":"output_text","text":"safe"}]}]}`
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
		}),
		refreshTransport: http.DefaultTransport,
		pool:             newProviderPool([]*ProviderConnection{connection}, false),
		registry:         NewProviderRegistry(NewCodexProvider(base, base, base), NewClaudeProvider(base), NewGeminiProvider(base, base)),
		metrics:          newMetrics(), recent: newRecentErrors(5),
	}
	body := `{"model":"gpt-5.5","stream":false,"input":[{"type":"message"},{"type":"mcp_approval_response","approval_request_id":"approval-secret"}],"tools":[{"type":"mcp","server_url":"https://private.example/mcp"},{"type":"web_search"},{"type":"function","name":"local_mcp_tool"}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("hosted-mcp-test", "user"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !upstreamCalled || response.Code != http.StatusOK {
		t.Fatalf("called=%v status=%d body=%s", upstreamCalled, response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"mcp_call", "TOP-SECRET"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Errorf("client response leaked %q: %s", forbidden, response.Body.String())
		}
	}
	if !strings.Contains(response.Body.String(), "web_search_call") || !strings.Contains(response.Body.String(), "safe") {
		t.Fatalf("safe response removed: %s", response.Body.String())
	}
}

func TestCodexProxyRejectsOversizedResponsesRequestBeforeTransport(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "hosted-mcp-test")
	base, _ := url.Parse("https://chatgpt.com/backend-api")
	called := false
	handler := &proxyHandler{
		cfg:       &config{maxAttempts: 1, maxInMemoryBodyBytes: 32},
		transport: roundTripFunc(func(*http.Request) (*http.Response, error) { called = true; return nil, nil }),
		pool:      newProviderPool([]*ProviderConnection{{Type: AccountTypeCodex, ID: "codex", AccessToken: "token", PlanType: "pro"}}, false),
		registry:  NewProviderRegistry(NewCodexProvider(base, base, base), NewClaudeProvider(base), NewGeminiProvider(base, base)), metrics: newMetrics(), recent: newRecentErrors(5),
	}
	body := []byte(`{"model":"gpt-5.5","input":"` + strings.Repeat("x", 80) + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("hosted-mcp-test", "user"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if called || response.Code != http.StatusBadRequest {
		t.Fatalf("transport called=%v status=%d body=%s", called, response.Code, response.Body.String())
	}
}

func TestHostedMCPWebSocketFilteringUsesConfiguredBounds(t *testing.T) {
	if got := effectiveWebSocketReadLimit(AccountTypeCodex, 2*1024*1024); got != 2*1024*1024 {
		t.Fatalf("Codex websocket read limit=%d, want configured bound", got)
	}
	state := &codexRelayState{h: &proxyHandler{cfg: &config{maxInMemoryBodyBytes: 32}}, opts: codexCyberSwapOptions{Provider: &CodexProvider{}}, activeAccount: &ProviderConnection{ID: "codex"}}
	frame := []byte(`{"type":"response.create","model":"gpt-5.5","input":"` + strings.Repeat("x", 64) + `"}`)
	if _, err := state.inspectClient(frame); err == nil {
		t.Fatal("oversized websocket transformation accepted")
	}
}

func TestCodexRelayStateFiltersHostedMCPFrames(t *testing.T) {
	state := &codexRelayState{h: &proxyHandler{cfg: &config{maxInMemoryBodyBytes: 1024}}, opts: codexCyberSwapOptions{Provider: &CodexProvider{}}, activeAccount: &ProviderConnection{ID: "codex"}}
	client := []byte(`{"type":"response.create","model":"gpt-5.5","tools":[{"type":"mcp","server_url":"private"},{"type":"function","name":"local_mcp_tool"}]}`)
	filtered, err := state.inspectClient(client)
	if err != nil || strings.Contains(string(filtered), `"type":"mcp"`) || !strings.Contains(string(filtered), "local_mcp_tool") {
		t.Fatalf("client filtered=%s err=%v", filtered, err)
	}
	upstream := []byte(`{"type":"response.output_item.done","item":{"type":"mcp_call","output":"secret"}}`)
	filtered, err = state.inspectUpstream(upstream)
	if err != nil || filtered != nil {
		t.Fatalf("upstream filtered=%s err=%v", filtered, err)
	}
}
