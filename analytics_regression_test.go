package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// TestDeepSeekSSEStreamRecordsExactlyOneRequest verifies that pass-through SSE
// for DeepSeek (Anthropic-compatible) correctly accumulates message_start +
// message_delta into a single RequestUsage record (account.Totals.RequestCount
// increments by exactly 1), rather than recording two separate rows.
func TestDeepSeekSSEStreamRecordsExactlyOneRequest(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "test-secret")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		// message_start with input tokens
		start := map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":      "msg_test",
				"type":    "message",
				"role":    "assistant",
				"model":   "deepseek-v4-pro",
				"content": []any{},
				"usage": map[string]any{
					"input_tokens":            float64(42),
					"cache_read_input_tokens": float64(5),
				},
			},
		}
		startData, _ := json.Marshal(start)
		_, _ = w.Write([]byte("event: message_start\ndata: " + string(startData) + "\n\n"))
		w.(http.Flusher).Flush()

		// content_delta
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"))
		w.(http.Flusher).Flush()

		// message_delta with output tokens
		delta := map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   "end_turn",
				"stop_sequence": nil,
			},
			"usage": map[string]any{
				"output_tokens": float64(99),
			},
		}
		deltaData, _ := json.Marshal(delta)
		_, _ = w.Write([]byte("event: message_delta\ndata: " + string(deltaData) + "\n\n"))

		// message_stop
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	baseURL, _ := url.Parse(upstream.URL)
	codex := NewCodexProvider(baseURL, baseURL, baseURL)
	claude := NewClaudeProvider(baseURL)
	gemini := NewGeminiProvider(baseURL, baseURL)
	deepseek := NewDeepSeekProvider(baseURL)
	registry := NewProviderRegistry(codex, claude, gemini, deepseek)

	acc := &Account{
		Type:        AccountTypeDeepSeek,
		ID:          "deepseek_test",
		AccessToken: "ds-test",
		PlanType:    "deepseek",
	}
	pool := newPoolState([]*Account{acc}, false)

	h := &proxyHandler{
		cfg: &config{
			requestTimeout:       10 * time.Second,
			streamTimeout:        10 * time.Second,
			maxInMemoryBodyBytes: 1024,
		},
		transport: http.DefaultTransport,
		pool:      pool,
		registry:  registry,
		metrics:   newMetrics(),
		recent:    newRecentErrors(5),
	}

	// Send a streaming request that routes to DeepSeek
	// Generate a valid pool JWT token
	poolToken := "Bearer " + generateClaudePoolToken("test-secret", "test-user")

	body := `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", poolToken)
	req.Header.Set("Accept", "text/event-stream")

	rr := httptest.NewRecorder()
	h.proxyRequest(rr, req, "test-req")

	// Check response is not corrupted
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	bodyOut := rr.Body.String()
	if !bytes.Contains([]byte(bodyOut), []byte("message_start")) {
		t.Fatal("response missing message_start")
	}
	if !bytes.Contains([]byte(bodyOut), []byte("message_delta")) {
		t.Fatal("response missing message_delta")
	}
	if !bytes.Contains([]byte(bodyOut), []byte("message_stop")) {
		t.Fatal("response missing message_stop")
	}

	// Check account totals: old broken behavior incremented RequestCount by 2
	// (once for message_start, once for message_delta).
	acc.mu.Lock()
	totals := acc.Totals
	acc.mu.Unlock()

	if totals.RequestCount != 1 {
		t.Fatalf("RequestCount = %d, want exactly 1 (old bug incremented by 2)", totals.RequestCount)
	}
	if totals.TotalInputTokens != 42 {
		t.Fatalf("TotalInputTokens = %d, want 42", totals.TotalInputTokens)
	}
	if totals.TotalCachedTokens != 5 {
		t.Fatalf("TotalCachedTokens = %d, want 5", totals.TotalCachedTokens)
	}
	if totals.TotalOutputTokens != 99 {
		t.Fatalf("TotalOutputTokens = %d, want 99", totals.TotalOutputTokens)
	}
	if totals.TotalBillableTokens != 136 {
		t.Fatalf("TotalBillableTokens = %d, want 136 (42-5+99)", totals.TotalBillableTokens)
	}
}

// TestZAISSEStreamRecordsExactlyOneRequest is the Z.ai equivalent.
func TestZAISSEStreamRecordsExactlyOneRequest(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "test-secret")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		start := map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":      "msg_test",
				"type":    "message",
				"role":    "assistant",
				"model":   "glm-5.2",
				"content": []any{},
				"usage": map[string]any{
					"input_tokens":  float64(0),
					"output_tokens": float64(0),
				},
			},
		}
		startData, _ := json.Marshal(start)
		_, _ = w.Write([]byte("event: message_start\ndata: " + string(startData) + "\n\n"))
		w.(http.Flusher).Flush()

		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"))
		w.(http.Flusher).Flush()

		delta := map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   "end_turn",
				"stop_sequence": nil,
			},
			"usage": map[string]any{
				"input_tokens":            float64(20),
				"cache_read_input_tokens": float64(3),
				"output_tokens":           float64(50),
			},
		}
		deltaData, _ := json.Marshal(delta)
		_, _ = w.Write([]byte("event: message_delta\ndata: " + string(deltaData) + "\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	baseURL, _ := url.Parse(upstream.URL)
	codex := NewCodexProvider(baseURL, baseURL, baseURL)
	claude := NewClaudeProvider(baseURL)
	gemini := NewGeminiProvider(baseURL, baseURL)
	zai := NewZAIProvider(baseURL)
	registry := NewProviderRegistry(codex, claude, gemini, zai)

	acc := &Account{
		Type:        AccountTypeZAI,
		ID:          "zai_test",
		AccessToken: "zai-test",
		PlanType:    "zai",
	}
	pool := newPoolState([]*Account{acc}, false)

	h := &proxyHandler{
		cfg: &config{
			requestTimeout:       10 * time.Second,
			streamTimeout:        10 * time.Second,
			maxInMemoryBodyBytes: 1024,
		},
		transport: http.DefaultTransport,
		pool:      pool,
		registry:  registry,
		metrics:   newMetrics(),
		recent:    newRecentErrors(5),
	}

	// Generate a valid pool JWT token
	poolToken := "Bearer " + generateClaudePoolToken("test-secret", "test-user")

	body := `{"model":"glm-5.2","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", poolToken)
	req.Header.Set("Accept", "text/event-stream")

	rr := httptest.NewRecorder()
	h.proxyRequest(rr, req, "test-req-zai")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("message_delta")) {
		t.Fatal("response missing message_delta")
	}

	acc.mu.Lock()
	totals := acc.Totals
	acc.mu.Unlock()

	if totals.RequestCount != 1 {
		t.Fatalf("RequestCount = %d, want exactly 1 (old bug incremented by 2)", totals.RequestCount)
	}
	if totals.TotalInputTokens != 20 {
		t.Fatalf("TotalInputTokens = %d, want 20", totals.TotalInputTokens)
	}
	if totals.TotalCachedTokens != 3 {
		t.Fatalf("TotalCachedTokens = %d, want 3", totals.TotalCachedTokens)
	}
	if totals.TotalOutputTokens != 50 {
		t.Fatalf("TotalOutputTokens = %d, want 50", totals.TotalOutputTokens)
	}
	if totals.TotalBillableTokens != 67 {
		t.Fatalf("TotalBillableTokens = %d, want 67 (20-3+50)", totals.TotalBillableTokens)
	}
}

// TestDeepSeekNonStreamingRecordsExactlyOneRequest verifies that a standard
// Anthropic JSON response (non-SSE) for DeepSeek records usage exactly once.
func TestDeepSeekNonStreamingRecordsExactlyOneRequest(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "test-secret")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		body := map[string]any{
			"id":      "msg_test",
			"type":    "message",
			"role":    "assistant",
			"model":   "deepseek-v4-pro",
			"content": []any{map[string]any{"type": "text", "text": "Hello"}},
			"usage": map[string]any{
				"input_tokens":  float64(10),
				"output_tokens": float64(20),
			},
		}
		data, _ := json.Marshal(body)
		_, _ = w.Write(data)
	}))
	defer upstream.Close()

	baseURL, _ := url.Parse(upstream.URL)
	codex := NewCodexProvider(baseURL, baseURL, baseURL)
	claude := NewClaudeProvider(baseURL)
	gemini := NewGeminiProvider(baseURL, baseURL)
	deepseek := NewDeepSeekProvider(baseURL)
	registry := NewProviderRegistry(codex, claude, gemini, deepseek)

	acc := &Account{
		Type:        AccountTypeDeepSeek,
		ID:          "deepseek_test_ns",
		AccessToken: "ds-test",
		PlanType:    "deepseek",
	}
	pool := newPoolState([]*Account{acc}, false)

	h := &proxyHandler{
		cfg: &config{
			requestTimeout:       10 * time.Second,
			streamTimeout:        10 * time.Second,
			maxInMemoryBodyBytes: 1024,
		},
		transport: http.DefaultTransport,
		pool:      pool,
		registry:  registry,
		metrics:   newMetrics(),
		recent:    newRecentErrors(5),
	}

	// Generate a valid pool JWT token
	poolToken := "Bearer " + generateClaudePoolToken("test-secret", "test-user")

	// Non-streaming request
	body := `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", poolToken)

	rr := httptest.NewRecorder()
	h.proxyRequest(rr, req, "test-req-ds-ns")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	acc.mu.Lock()
	totals := acc.Totals
	acc.mu.Unlock()

	if totals.RequestCount != 1 {
		t.Fatalf("RequestCount = %d, want exactly 1", totals.RequestCount)
	}
	if totals.TotalInputTokens != 10 {
		t.Fatalf("TotalInputTokens = %d, want 10", totals.TotalInputTokens)
	}
	if totals.TotalOutputTokens != 20 {
		t.Fatalf("TotalOutputTokens = %d, want 20", totals.TotalOutputTokens)
	}
	if totals.TotalBillableTokens != 30 {
		t.Fatalf("TotalBillableTokens = %d, want 30 (10+20)", totals.TotalBillableTokens)
	}
}
