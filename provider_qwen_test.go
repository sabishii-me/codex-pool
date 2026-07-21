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
)

func TestIsQwenModelHandlesCatalogModels(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"qwen3.6-plus", "QWEN3.6-PLUS", "qwen", "qwen-coder", "qwen3-coder"} {
		if !isQwenModel(model) {
			t.Fatalf("expected %q to route to qwen", model)
		}
	}

	for _, model := range []string{"qwen3.5-plus", "gpt-5.6", "kimi-for-coding"} {
		if isQwenModel(model) {
			t.Fatalf("did not expect %q to route to qwen", model)
		}
	}
}

func TestModelRouteOverrideQwenModelUsesQwenBase(t *testing.T) {
	t.Parallel()

	qwenBase, _ := url.Parse("https://coding-intl.dashscope.aliyuncs.com/apps/anthropic")
	handler := &proxyHandler{
		registry: NewProviderRegistry(
			&CodexProvider{},
			&ClaudeProvider{},
			&GeminiProvider{},
			NewQwenProvider(qwenBase),
		),
	}

	provider, base, rewritten := handler.modelRouteOverride("/v1/messages", "qwen-coder", []byte(`{"model":"qwen-coder"}`))
	if provider == nil {
		t.Fatal("expected override provider")
	}
	if provider.Type() != AccountTypeQwen {
		t.Fatalf("expected qwen provider, got %s", provider.Type())
	}
	if base == nil || base.String() != qwenBase.String() {
		t.Fatalf("expected qwen base %s, got %v", qwenBase, base)
	}
	if string(rewritten) != `{"model":"qwen3.6-plus"}` {
		t.Fatalf("unexpected rewritten body: %s", rewritten)
	}
}

func TestQwenAdminAddValidatesAndSavesAccount(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	qwenBase, _ := url.Parse("https://coding-intl.dashscope.aliyuncs.com/apps/anthropic")
	validationCalled := false
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir, qwenBase: qwenBase},
		pool:     newPoolState(nil, false),
		registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewQwenProvider(qwenBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			validationCalled = true
			if req.Header.Get("Authorization") != "Bearer qw-valid" {
				t.Fatalf("validation auth = %q", req.Header.Get("Authorization"))
			}
			var body map[string]any
			payload, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(payload, &body)
			if body["model"] != "qwen3.6-plus" {
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

	req := httptest.NewRequest(http.MethodPost, "/admin/qwen/add", strings.NewReader(`{"api_key":"qw-valid"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleQwenAdd(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !validationCalled {
		t.Fatal("validation was not called")
	}
	entries, err := os.ReadDir(filepath.Join(poolDir, "qwen"))
	if err != nil {
		t.Fatalf("read qwen pool dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("saved %d Qwen files, want 1", len(entries))
	}
	if h.pool.countByType(AccountTypeQwen) != 1 {
		t.Fatalf("pool Qwen count = %d, want 1", h.pool.countByType(AccountTypeQwen))
	}
}

func TestQwenAdminRejectsUnauthorizedKeyWithoutSaving(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	qwenBase, _ := url.Parse("https://coding-intl.dashscope.aliyuncs.com/apps/anthropic")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir, qwenBase: qwenBase},
		pool:     newPoolState(nil, false),
		registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewQwenProvider(qwenBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader(`{"error":"bad key"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/qwen/add", strings.NewReader(`{"api_key":"qw-bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleQwenAdd(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(poolDir, "qwen")); !os.IsNotExist(err) {
		t.Fatalf("qwen pool dir should not exist after rejected key, err=%v", err)
	}
}
