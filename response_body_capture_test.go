package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

func TestResponseBodyCaptureRetainsBoundedPrefixAndTail(t *testing.T) {
	capture := newResponseBodyCapture(8)
	input := []byte("0123456789abcdef")
	for _, chunk := range [][]byte{input[:3], input[3:11], input[11:]} {
		if _, err := capture.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if got := string(capture.Bytes()); got != "01234567" {
		t.Fatalf("prefix=%q", got)
	}
	if got := string(capture.TailBytes()); got != "89abcdef" {
		t.Fatalf("tail=%q", got)
	}
	if !capture.Truncated() {
		t.Fatal("capture did not report truncation")
	}
}

func TestProtocolUsageObjectFromJSONTail(t *testing.T) {
	tail := []byte(`discarded output... "usage":{"input_tokens":22865,"cache_read_input_tokens":30,"output_tokens":6000}}`)
	object := protocolUsageObjectFromJSONTail(tail, "deepseek-v4-pro")
	if object == nil || object["model"] != "deepseek-v4-pro" {
		t.Fatalf("object=%#v", object)
	}
	usage, _ := object["usage"].(map[string]any)
	if toInt64(usage["input_tokens"]) != 22865 || toInt64(usage["output_tokens"]) != 6000 {
		t.Fatalf("usage=%#v", usage)
	}
}

func TestLargeNonStreamingDeepSeekResponseRecordsUsageExactlyOnce(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "large-response-contract-secret")
	responseObject := map[string]any{
		"id": "msg_large", "type": "message", "role": "assistant", "model": "deepseek-v4-pro",
		"content": []any{map[string]any{"type": "text", "text": string(bytes.Repeat([]byte("x"), 64*1024))}},
		"usage": map[string]any{
			"input_tokens": 22865, "cache_read_input_tokens": 30,
			"cache_creation_input_tokens": 20, "output_tokens": 6000,
		},
	}
	responseBody, err := json.Marshal(responseObject)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(responseBody)
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	registry := anthropicContractRegistry(base)
	connection := &ProviderConnection{Type: AccountTypeDeepSeek, ID: "deepseek_large_response", AccessToken: "contract-key", PlanType: "deepseek"}
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.db.Close()
	handler := &proxyHandler{
		cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
		transport: http.DefaultTransport, pool: newProviderPool([]*ProviderConnection{connection}), registry: registry,
		analyticsStore: analytics, metrics: newMetrics(), recent: newRecentErrors(5),
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"review"}],"stream":false}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("large-response-contract-secret", "review-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "large-deepseek-review")
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), responseBody) {
		t.Fatalf("response changed: status=%d got=%d want=%d", response.Code, response.Body.Len(), len(responseBody))
	}
	assertCanonicalCustomUsage(t, analytics, connection, "large-deepseek-review", "review-user", 22865, 30, 20, 6000, 0, 28815)
}
