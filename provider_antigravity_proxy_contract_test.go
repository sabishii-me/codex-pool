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

func TestAntigravityProxyNonStreamingResponsesCanonicalUsage(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	model := "gemini-3-flash"
	upstreamSSE := []byte(`data: {"response":{"responseId":"native-contract","candidates":[{"content":{"role":"model","parts":[{"thought":true,"thoughtSignature":"signature","text":"reason"},{"text":"unchanged"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":120,"cachedContentTokenCount":30,"candidatesTokenCount":40,"thoughtsTokenCount":7,"totalTokenCount":167}}}` + "\n\n")
	var forwarded []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		forwarded, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(upstreamSSE)
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	antigravity := NewAntigravityProvider(base, base)
	registry := NewProviderRegistry(NewCodexProvider(base, base, base), NewClaudeProvider(base), NewGeminiProvider(base, base), antigravity)
	account := &Account{Type: AccountTypeAntigravity, ID: "antigravity_contract", AccessToken: "contract-key", ProjectID: "project-contract", PlanType: "antigravity", ModelRateLimits: make(map[string]time.Time)}
	antigravityModels.ReplaceAccount(account.ID, AntigravityAccountSnapshot{FetchedAt: time.Now(), Models: map[string]AntigravityModelInfo{model: {ID: model}}})
	defer antigravityModels.ReplaceAccount(account.ID, AntigravityAccountSnapshot{})
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.db.Close()
	handler := &proxyHandler{cfg: &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024, maxAttempts: 1, disableRefresh: true}, transport: http.DefaultTransport, antigravityTransport: http.DefaultTransport, pool: newProviderPool([]*Account{account}, false), registry: registry, analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil)}
	requestBody := []byte(`{"model":"antigravity/gemini-3-flash","input":"hello","stream":false}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	requestID := "antigravity-contract"
	handler.proxyRequest(response, request, requestID)
	if response.Code != http.StatusOK {
		t.Fatalf("response status=%d body=%s", response.Code, response.Body.String())
	}
	var public map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	if public["object"] != "response" || public["model"] != "antigravity/gemini-3-flash" || public["status"] != "completed" {
		t.Fatalf("translated response metadata = %#v", public)
	}
	if !strings.Contains(response.Body.String(), "unchanged") || !strings.Contains(response.Body.String(), "reason") {
		t.Fatalf("translated response lost content: %s", response.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(forwarded, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["project"] != "project-contract" || envelope["model"] != model {
		t.Fatalf("upstream envelope metadata = %#v", envelope)
	}
	assertCanonicalCustomUsage(t, analytics, account, requestID, "contract-user", 120, 30, 0, 40, 7, 130)
}

func TestAntigravityLargeBodyIsRejectedBeforeUpstream(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	antigravity := NewAntigravityProvider(base, base)
	registry := NewProviderRegistry(NewCodexProvider(base, base, base), NewClaudeProvider(base), NewGeminiProvider(base, base), antigravity)
	account := &Account{Type: AccountTypeAntigravity, ID: "antigravity_large", AccessToken: "contract", ProjectID: "project", ModelRateLimits: make(map[string]time.Time)}
	handler := &proxyHandler{cfg: &config{maxInMemoryBodyBytes: 1024, maxAttempts: 1}, transport: http.DefaultTransport, pool: newProviderPool([]*Account{account}, false), registry: registry, metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil)}
	body := []byte(`{"model":"antigravity/gemini-3-flash","input":` + mustJSONContractString(t, strings.Repeat("x", streamedModelRoutePeekBytes+1024)) + `,"stream":false}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "antigravity-large-contract")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "requires full-body translation or sanitization") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if calls != 0 {
		t.Fatalf("upstream calls = %d, want 0", calls)
	}
}
