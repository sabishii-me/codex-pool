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

// NVIDIA is an aggregator like OpenRouter: routing is by explicit "nvidia/"
// prefix, not a fixed catalog.
func TestIsNvidiaModelRequiresPrefix(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"nvidia/meta/llama-3.3-70b-instruct", "nvidia/qwen/qwen3-coder-480b-a35b-instruct"} {
		if !isNvidiaModel(model) {
			t.Fatalf("expected %q to route to nvidia", model)
		}
	}

	for _, model := range []string{"meta/llama-3.3-70b-instruct", "gpt-5.6", "openrouter/meta/llama-3.3-70b-instruct"} {
		if isNvidiaModel(model) {
			t.Fatalf("did not expect %q to route to nvidia (no explicit prefix)", model)
		}
	}
}

func TestNvidiaCanonicalModelStripsPrefix(t *testing.T) {
	t.Parallel()

	got := nvidiaCanonicalModel("nvidia/meta/llama-3.3-70b-instruct")
	want := "meta/llama-3.3-70b-instruct"
	if got != want {
		t.Fatalf("nvidiaCanonicalModel() = %q, want %q", got, want)
	}
}

func TestModelRouteOverrideNvidiaModelStripsPrefixAndUsesNvidiaBase(t *testing.T) {
	t.Parallel()

	nvidiaBase, _ := url.Parse("https://integrate.api.nvidia.com/v1")
	handler := &proxyHandler{
		registry: NewProviderRegistry(
			&CodexProvider{},
			&ClaudeProvider{},
			&GeminiProvider{},
			NewNvidiaProvider(nvidiaBase),
		),
	}

	provider, base, rewritten := handler.modelRouteOverride("/v1/messages", "nvidia/meta/llama-3.3-70b-instruct", []byte(`{"model":"nvidia/meta/llama-3.3-70b-instruct"}`))
	if provider == nil {
		t.Fatal("expected override provider")
	}
	if provider.Type() != AccountTypeNvidia {
		t.Fatalf("expected nvidia provider, got %s", provider.Type())
	}
	if base == nil || base.String() != nvidiaBase.String() {
		t.Fatalf("expected nvidia base %s, got %v", nvidiaBase, base)
	}
	if string(rewritten) != `{"model":"meta/llama-3.3-70b-instruct"}` {
		t.Fatalf("unexpected rewritten body (prefix should be stripped): %s", rewritten)
	}
}

func TestNvidiaProviderTargetsOpenAIFormat(t *testing.T) {
	t.Parallel()

	if got := providerTargetFormat(AccountTypeNvidia); got != FormatOpenAI {
		t.Fatalf("providerTargetFormat(nvidia) = %v, want FormatOpenAI", got)
	}
}

func TestNvidiaParseUsageReadsOpenAIShape(t *testing.T) {
	t.Parallel()

	p := NewNvidiaProvider(nil)

	// Non-streaming OpenAI Chat Completions response shape.
	ru := p.ParseUsage(map[string]any{
		"model": "meta/llama-3.3-70b-instruct",
		"usage": map[string]any{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(50),
			"total_tokens":      float64(150),
		},
	})
	if ru == nil {
		t.Fatal("expected usage, got nil")
	}
	if ru.InputTokens != 100 || ru.OutputTokens != 50 || ru.BillableTokens != 150 {
		t.Fatalf("unexpected usage: %+v", ru)
	}
	if ru.Model != "meta/llama-3.3-70b-instruct" {
		t.Fatalf("unexpected model: %q", ru.Model)
	}

	// A chunk with no usage (mid-stream) must return nil, not zero-value usage.
	if got := p.ParseUsage(map[string]any{"choices": []any{}}); got != nil {
		t.Fatalf("expected nil for chunk without usage, got %+v", got)
	}
}

func TestNvidiaAdminAddValidatesUsingOpenAIChatCompletionsShape(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	nvidiaBase, _ := url.Parse("https://integrate.api.nvidia.com/v1")
	validationCalled := false
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir, nvidiaBase: nvidiaBase},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewNvidiaProvider(nvidiaBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			validationCalled = true
			if req.URL.String() != "https://integrate.api.nvidia.com/v1/chat/completions" {
				t.Fatalf("validation URL = %q", req.URL.String())
			}
			if req.Header.Get("Authorization") != "Bearer nvapi-valid" {
				t.Fatalf("validation auth = %q", req.Header.Get("Authorization"))
			}
			if req.Header.Get("anthropic-version") != "" {
				t.Fatal("nvidia validation must not send an anthropic-version header")
			}
			var body map[string]any
			payload, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(payload, &body)
			if _, ok := body["messages"]; !ok {
				t.Fatalf("expected OpenAI-shaped messages field, body=%v", body)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/nvidia/add", strings.NewReader(`{"api_key":"nvapi-valid"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleNvidiaAdd(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !validationCalled {
		t.Fatal("validation was not called")
	}
	entries, err := os.ReadDir(filepath.Join(poolDir, "nvidia"))
	if err != nil {
		t.Fatalf("read nvidia pool dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("saved %d NVIDIA files, want 1", len(entries))
	}
	if h.pool.countByType(AccountTypeNvidia) != 1 {
		t.Fatalf("pool NVIDIA count = %d, want 1", h.pool.countByType(AccountTypeNvidia))
	}
}

func TestNvidiaAdminRejectsUnauthorizedKeyWithoutSaving(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	nvidiaBase, _ := url.Parse("https://integrate.api.nvidia.com/v1")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir, nvidiaBase: nvidiaBase},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewNvidiaProvider(nvidiaBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader(`{"error":"bad key"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/nvidia/add", strings.NewReader(`{"api_key":"nvapi-bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleNvidiaAdd(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(poolDir, "nvidia")); !os.IsNotExist(err) {
		t.Fatalf("nvidia pool dir should not exist after rejected key, err=%v", err)
	}
}
