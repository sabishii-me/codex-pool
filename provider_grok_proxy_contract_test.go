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

func TestGrokProxyCanonicalUsageAndSanitizedResponseIntegrity(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	responseBody := []byte(`{"type":"response.completed","response":{"id":"resp_contract","model":"grok-composer-2.5-fast","output":[],"usage":{"input_tokens":120,"output_tokens":40,"input_tokens_details":{"cached_tokens":30},"output_tokens_details":{"reasoning_tokens":7}}}}`)
	var forwarded []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		forwarded, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(responseBody)
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	grok := NewGrokProvider(base)
	registry := NewProviderRegistry(NewCodexProvider(base, base, base), NewClaudeProvider(base), NewGeminiProvider(base, base), grok)
	account := &Account{Type: AccountTypeGrok, ID: "grok_contract", AccessToken: "contract-key", PlanType: "grok"}
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.db.Close()
	handler := &proxyHandler{
		cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
		transport: http.DefaultTransport, pool: newPoolState([]*Account{account}, false), registry: registry,
		analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
	}
	requestBody := []byte(`{"model":"grok-composer","input":"hello","stream":false,"metadata":{"conversation_id":"remove"},"reasoning":{"effort":"high"}}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	requestID := "grok-contract"
	handler.proxyRequest(response, request, requestID)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), responseBody) {
		t.Fatalf("response status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(forwarded, &body); err != nil {
		t.Fatal(err)
	}
	if body["model"] != "grok-composer-2.5-fast" {
		t.Fatalf("forwarded model = %v", body["model"])
	}
	for _, forbidden := range []string{"metadata", "reasoning"} {
		if _, exists := body[forbidden]; exists {
			t.Fatalf("forwarded body retained %s: %s", forbidden, forwarded)
		}
	}
	assertCanonicalCustomUsage(t, analytics, account, requestID, "contract-user", 120, 30, 0, 40, 7, 130)
}

func TestGrokLargeBodyIsRejectedBeforeUpstream(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	registry := NewProviderRegistry(NewCodexProvider(base, base, base), NewClaudeProvider(base), NewGeminiProvider(base, base), NewGrokProvider(base))
	account := &Account{Type: AccountTypeGrok, ID: "grok_large", AccessToken: "contract-key"}
	handler := &proxyHandler{cfg: &config{maxInMemoryBodyBytes: 1024}, transport: http.DefaultTransport, pool: newPoolState([]*Account{account}, false), registry: registry, metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil)}
	body := []byte(`{"model":"grok-composer","input":` + mustJSONContractString(t, strings.Repeat("x", streamedModelRoutePeekBytes+1024)) + `,"metadata":{"must":"not leak"}}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "grok-large-contract")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "requires full-body translation or sanitization") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if calls != 0 {
		t.Fatalf("upstream calls = %d, want 0", calls)
	}
}

func assertCanonicalCustomUsage(t *testing.T, analytics *AnalyticsStore, account *Account, requestID, userID string, input, cacheRead, cacheWrite, output, reasoning, billable int64) {
	t.Helper()
	var gotCount, gotInput, gotCacheRead, gotCacheWrite, gotOutput, gotReasoning, gotBillable int64
	var gotRequestID, gotUserID, gotProviderID, gotConnectionID string
	err := analytics.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_write_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(reasoning_tokens),0), COALESCE(SUM(billable_tokens),0), COALESCE(MAX(request_id),''), COALESCE(MAX(user_id),''), COALESCE(MAX(provider_id),''), COALESCE(MAX(connection_id),'') FROM usage_events WHERE connection_id = ?`, account.ID).Scan(&gotCount, &gotInput, &gotCacheRead, &gotCacheWrite, &gotOutput, &gotReasoning, &gotBillable, &gotRequestID, &gotUserID, &gotProviderID, &gotConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if gotCount != 1 || gotInput != input || gotCacheRead != cacheRead || gotCacheWrite != cacheWrite || gotOutput != output || gotReasoning != reasoning || gotBillable != billable || gotRequestID != requestID || gotUserID != userID || gotProviderID != string(account.Type) || gotConnectionID != account.ID {
		t.Fatalf("canonical custom usage count=%d input=%d cache-read=%d cache-write=%d output=%d reasoning=%d billable=%d request=%q user=%q provider=%q connection=%q", gotCount, gotInput, gotCacheRead, gotCacheWrite, gotOutput, gotReasoning, gotBillable, gotRequestID, gotUserID, gotProviderID, gotConnectionID)
	}
}
