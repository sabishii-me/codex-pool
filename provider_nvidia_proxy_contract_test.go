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

func TestNvidiaProxyCanonicalUsageAndLargeBodyIntegrity(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	responseBody := []byte(`{"id":"chatcmpl-contract","object":"chat.completion","model":"meta/llama-3.3-70b-instruct","choices":[{"index":0,"message":{"role":"assistant","content":"unchanged"},"finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":40,"prompt_tokens_details":{"cached_tokens":30},"completion_tokens_details":{"reasoning_tokens":7}}}`)
	for _, large := range []bool{false, true} {
		name := "small"
		padding := "hello"
		if large {
			name = "large"
			padding = strings.Repeat("x", streamedModelRoutePeekBytes+1024)
		}
		t.Run(name, func(t *testing.T) {
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
			nvidia := NewNvidiaProvider(base)
			registry := NewProviderRegistry(NewCodexProvider(base, base, base), NewGeminiProvider(base, base), nvidia)
			account := &Account{Type: AccountTypeNvidia, ID: "nvidia_" + name, AccessToken: "contract-key", PlanType: "nvidia"}
			analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer analytics.db.Close()
			handler := &proxyHandler{
				cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024},
				transport: http.DefaultTransport, pool: newProviderPool([]*Account{account}), registry: registry,
				analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
			}
			requestBody := []byte(`{"model":"nvidia/meta/llama-3.3-70b-instruct","messages":[{"role":"user","content":` + mustJSONContractString(t, padding) + `}],"stream":false}`)
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestBody))
			request.ContentLength = int64(len(requestBody))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
			response := httptest.NewRecorder()
			requestID := "nvidia-contract-" + name
			handler.proxyRequest(response, request, requestID)
			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), responseBody) {
				t.Fatalf("response status=%d body=%s", response.Code, response.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(forwarded, &body); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "meta/llama-3.3-70b-instruct" {
				t.Fatalf("forwarded model = %v", body["model"])
			}
			messages := body["messages"].([]any)
			if got := messages[0].(map[string]any)["content"]; got != padding {
				t.Fatalf("forwarded content changed: got %d bytes, want %d", len(got.(string)), len(padding))
			}
			assertCanonicalNvidiaUsage(t, analytics, account, requestID)
		})
	}
}

func mustJSONContractString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func assertCanonicalNvidiaUsage(t *testing.T, analytics *AnalyticsStore, account *Account, requestID string) {
	t.Helper()
	var count, input, cacheRead, cacheWrite, output, reasoning, billable int64
	var persistedRequestID, userID, providerID, connectionID string
	err := analytics.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(cache_read_tokens),0),
		COALESCE(SUM(cache_write_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(reasoning_tokens),0),
		COALESCE(SUM(billable_tokens),0), COALESCE(MAX(request_id),''), COALESCE(MAX(user_id),''),
		COALESCE(MAX(provider_id),''), COALESCE(MAX(connection_id),'') FROM usage_events WHERE connection_id = ?`, account.ID).Scan(
		&count, &input, &cacheRead, &cacheWrite, &output, &reasoning, &billable,
		&persistedRequestID, &userID, &providerID, &connectionID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || input != 120 || cacheRead != 30 || cacheWrite != 0 || output != 40 || reasoning != 7 || billable != 130 || persistedRequestID != requestID || userID != "contract-user" || providerID != "nvidia" || connectionID != account.ID {
		t.Fatalf("canonical NVIDIA usage count=%d input=%d cache-read=%d cache-write=%d output=%d reasoning=%d billable=%d request=%q user=%q provider=%q connection=%q",
			count, input, cacheRead, cacheWrite, output, reasoning, billable, persistedRequestID, userID, providerID, connectionID)
	}
}
