package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsDeepSeekModelHandlesCatalogModels(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"deepseek-v4-flash", "DEEPSEEK-V4-PRO", "deepseek", "deepseek-pro"} {
		if !isDeepSeekModel(model) {
			t.Fatalf("expected %q to route to deepseek", model)
		}
	}

	for _, model := range []string{"deepseek-v3", "deepseek-chat", "gpt-5.6"} {
		if isDeepSeekModel(model) {
			t.Fatalf("did not expect %q to route to deepseek", model)
		}
	}
}

func TestModelRouteOverrideDeepSeekModelUsesDeepSeekBase(t *testing.T) {
	t.Parallel()

	deepseekBase, _ := url.Parse("https://api.deepseek.com/anthropic")
	handler := &proxyHandler{
		registry: NewProviderRegistry(
			&CodexProvider{},
			&GeminiProvider{},
			NewDeepSeekProvider(deepseekBase),
		),
	}

	provider, base, rewritten := handler.modelRouteOverride("/v1/messages", "deepseek", []byte(`{"model":"deepseek"}`))
	if provider == nil {
		t.Fatal("expected override provider")
	}
	if provider.Type() != AccountTypeDeepSeek {
		t.Fatalf("expected deepseek provider, got %s", provider.Type())
	}
	if base == nil || base.String() != deepseekBase.String() {
		t.Fatalf("expected deepseek base %s, got %v", deepseekBase, base)
	}
	if string(rewritten) != `{"model":"deepseek-v4-pro"}` {
		t.Fatalf("unexpected rewritten body: %s", rewritten)
	}
}

func TestDeepSeekProviderUsesBearerAuth(t *testing.T) {
	t.Parallel()

	p := NewDeepSeekProvider(nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	p.SetAuthHeaders(req, &Account{AccessToken: "sk-test"})
	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer sk-test")
	}
	if req.Header.Get("X-Api-Key") != "" {
		t.Fatal("DeepSeek should not set X-Api-Key")
	}
}

func TestDeepSeekAdminAddValidatesAndSavesAccount(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	deepseekBase, _ := url.Parse("https://api.deepseek.com/anthropic")
	validationCalled := false
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir, deepseekBase: deepseekBase},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewDeepSeekProvider(deepseekBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			validationCalled = true
			if req.URL.String() != "https://api.deepseek.com/anthropic/v1/messages" {
				t.Fatalf("validation URL = %q", req.URL.String())
			}
			if req.Header.Get("Authorization") != "Bearer ds-valid" {
				t.Fatalf("validation auth = %q", req.Header.Get("Authorization"))
			}
			var body map[string]any
			payload, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(payload, &body)
			if body["model"] != "deepseek-v4-flash" {
				t.Fatalf("validation model = %v", body["model"])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/deepseek/add", strings.NewReader(`{"api_key":"ds-valid"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleDeepSeekAdd(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !validationCalled {
		t.Fatal("validation was not called")
	}
	entries, err := os.ReadDir(filepath.Join(poolDir, "deepseek"))
	if err != nil {
		t.Fatalf("read deepseek pool dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("saved %d DeepSeek files, want 1", len(entries))
	}
	if h.pool.countByType(AccountTypeDeepSeek) != 1 {
		t.Fatalf("pool DeepSeek count = %d, want 1", h.pool.countByType(AccountTypeDeepSeek))
	}
}

func TestDeepSeekUsagePollerSkipsGenericFetch(t *testing.T) {
	t.Parallel()

	accountFile := filepath.Join(t.TempDir(), "deepseek.json")
	if err := os.WriteFile(accountFile, []byte(`{"api_key":"ds-valid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	acc := &Account{
		Type:        AccountTypeDeepSeek,
		ID:          "deepseek",
		File:        accountFile,
		AccessToken: "ds-valid",
		PlanType:    "deepseek",
	}
	calls := 0
	h := &proxyHandler{
		cfg:  &config{usageRefresh: time.Minute},
		pool: newProviderPool([]*Account{acc}),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			t.Fatalf("DeepSeek usage poller should not call transport, got %s %s", req.Method, req.URL.String())
			return nil, nil
		}),
	}

	h.pollUpstreamUsage()

	if calls != 0 {
		t.Fatalf("transport calls = %d, want 0", calls)
	}
	if acc.Dead {
		t.Fatal("DeepSeek account was marked dead by usage poller")
	}
	data, err := os.ReadFile(accountFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"dead"`) {
		t.Fatalf("usage poller persisted dead flag for DeepSeek account: %s", data)
	}
}

func TestDeepSeekParseUsageMessageStart(t *testing.T) {
	t.Parallel()

	p := NewDeepSeekProvider(nil)

	// Full message_start with input, cached, model
	ru := p.ParseUsage(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"model": "deepseek-v4-pro",
			"usage": map[string]any{
				"input_tokens":            float64(42),
				"cache_read_input_tokens": float64(5),
			},
		},
	})
	if ru == nil {
		t.Fatal("expected non-nil usage")
	}
	if ru.InputTokens != 42 {
		t.Fatalf("InputTokens = %d, want 42", ru.InputTokens)
	}
	if ru.CachedInputTokens != 5 {
		t.Fatalf("CachedInputTokens = %d, want 5", ru.CachedInputTokens)
	}
	if ru.OutputTokens != 0 {
		t.Fatalf("OutputTokens = %d, want 0", ru.OutputTokens)
	}
	if ru.BillableTokens != 37 {
		t.Fatalf("BillableTokens = %d, want 37", ru.BillableTokens)
	}
	if ru.Model != "deepseek-v4-pro" {
		t.Fatalf("Model = %q, want deepseek-v4-pro", ru.Model)
	}

	// message_start with zero input tokens returns nil
	if got := p.ParseUsage(map[string]any{
		"type":    "message_start",
		"message": map[string]any{"usage": map[string]any{"input_tokens": float64(0)}},
	}); got != nil {
		t.Fatal("expected nil for zero input tokens")
	}

	// message_start without message key returns nil
	if got := p.ParseUsage(map[string]any{"type": "message_start"}); got != nil {
		t.Fatal("expected nil for missing message")
	}

	// message_start without usage returns nil
	if got := p.ParseUsage(map[string]any{"type": "message_start", "message": map[string]any{}}); got != nil {
		t.Fatal("expected nil for missing usage")
	}

	// Non-Anthropic event type returns nil
	if got := p.ParseUsage(map[string]any{"type": "ping"}); got != nil {
		t.Fatal("expected nil for ping event")
	}
}

func TestDeepSeekParseUsageMessageDelta(t *testing.T) {
	t.Parallel()

	p := NewDeepSeekProvider(nil)

	ru := p.ParseUsage(map[string]any{
		"type":  "message_delta",
		"usage": map[string]any{"output_tokens": float64(99)},
	})
	if ru == nil {
		t.Fatal("expected non-nil usage")
	}
	if ru.OutputTokens != 99 {
		t.Fatalf("OutputTokens = %d, want 99", ru.OutputTokens)
	}
	if ru.BillableTokens != 99 {
		t.Fatalf("BillableTokens = %d, want 99", ru.BillableTokens)
	}
	if ru.InputTokens != 0 {
		t.Fatalf("InputTokens = %d, want 0", ru.InputTokens)
	}

	// Zero output tokens returns nil
	if got := p.ParseUsage(map[string]any{
		"type":  "message_delta",
		"usage": map[string]any{"output_tokens": float64(0)},
	}); got != nil {
		t.Fatal("expected nil for zero output tokens")
	}

	// Missing usage returns nil
	if got := p.ParseUsage(map[string]any{"type": "message_delta"}); got != nil {
		t.Fatal("expected nil for missing usage")
	}
}

func TestDeepSeekAdminRejectsUnauthorizedKeyWithoutSaving(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	deepseekBase, _ := url.Parse("https://api.deepseek.com/anthropic")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir, deepseekBase: deepseekBase},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewDeepSeekProvider(deepseekBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader(`{"error":"bad key"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/deepseek/add", strings.NewReader(`{"api_key":"ds-bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleDeepSeekAdd(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(poolDir, "deepseek")); !os.IsNotExist(err) {
		t.Fatalf("deepseek pool dir should not exist after rejected key, err=%v", err)
	}
}
