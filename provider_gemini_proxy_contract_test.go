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

func TestGeminiProxyCanonicalUsageAndLargeBodyIntegrity(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	responseBody := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"unchanged"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":120,"cachedContentTokenCount":30,"candidatesTokenCount":40,"thoughtsTokenCount":7,"totalTokenCount":167},"modelVersion":"gemini-contract"}`)
	for _, large := range []bool{false, true} {
		name := "small"
		padding := "hello"
		maxBody := int64(1024 * 1024)
		if large {
			name = "large"
			padding = strings.Repeat("x", streamedModelRoutePeekBytes+1024)
			maxBody = 1024
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
			gemini := NewGeminiProvider(base, base)
			registry := NewProviderRegistry(NewCodexProvider(base, base, base), gemini)
			account := &Account{Type: AccountTypeGemini, ID: "gemini_" + name, AccessToken: "contract-key", PlanType: "gemini"}
			analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer analytics.db.Close()
			handler := &proxyHandler{
				cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: maxBody},
				transport: http.DefaultTransport, pool: newProviderPool([]*Account{account}), registry: registry,
				analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
			}
			requestBody := []byte(`{"contents":[{"role":"user","parts":[{"text":` + mustJSONContractString(t, padding) + `}]}]}`)
			path := "/v1beta/models/gemini-contract:generateContent"
			request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(requestBody))
			request.ContentLength = int64(len(requestBody))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
			response := httptest.NewRecorder()
			requestID := "gemini-contract-" + name
			handler.proxyRequest(response, request, requestID)
			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), responseBody) {
				t.Fatalf("response status=%d body=%s", response.Code, response.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(forwarded, &body); err != nil {
				t.Fatal(err)
			}
			contents := body["contents"].([]any)
			parts := contents[0].(map[string]any)["parts"].([]any)
			if got := parts[0].(map[string]any)["text"]; got != padding {
				t.Fatalf("forwarded content changed: got %d bytes, want %d", len(got.(string)), len(padding))
			}
			assertCanonicalCustomUsage(t, analytics, account, requestID, "contract-user", 120, 30, 0, 40, 7, 130)
		})
	}
}
