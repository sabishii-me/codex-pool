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

// OpenRouter is an aggregator: routing is by explicit "openrouter/" prefix,
// not a fixed catalog, since it fronts hundreds of vendor-prefixed models.
func TestIsOpenRouterModelRequiresPrefix(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"openrouter/anthropic/claude-opus-4.5", "openrouter/deepseek/deepseek-v4", "openrouter/x-ai/grok-4.5"} {
		if !isOpenRouterModel(model) {
			t.Fatalf("expected %q to route to openrouter", model)
		}
	}

	for _, model := range []string{"anthropic/claude-opus-4.5", "gpt-5.6", "deepseek-v4-pro"} {
		if isOpenRouterModel(model) {
			t.Fatalf("did not expect %q to route to openrouter (no explicit prefix)", model)
		}
	}
}

func TestOpenRouterCanonicalModelStripsPrefix(t *testing.T) {
	t.Parallel()

	got := openrouterCanonicalModel("openrouter/anthropic/claude-opus-4.5")
	want := "anthropic/claude-opus-4.5"
	if got != want {
		t.Fatalf("openrouterCanonicalModel() = %q, want %q", got, want)
	}
}

func TestModelRouteOverrideOpenRouterModelStripsPrefixAndUsesOpenRouterBase(t *testing.T) {
	t.Parallel()

	openrouterBase, _ := url.Parse("https://openrouter.ai/api")
	handler := &proxyHandler{
		registry: NewProviderRegistry(
			&CodexProvider{},
			&GeminiProvider{},
			NewOpenRouterProvider(openrouterBase),
		),
	}

	provider, base, rewritten := handler.modelRouteOverride("/v1/messages", "openrouter/anthropic/claude-opus-4.5", []byte(`{"model":"openrouter/anthropic/claude-opus-4.5"}`))
	if provider == nil {
		t.Fatal("expected override provider")
	}
	if provider.Type() != AccountTypeOpenRouter {
		t.Fatalf("expected openrouter provider, got %s", provider.Type())
	}
	if base == nil || base.String() != openrouterBase.String() {
		t.Fatalf("expected openrouter base %s, got %v", openrouterBase, base)
	}
	if string(rewritten) != `{"model":"anthropic/claude-opus-4.5"}` {
		t.Fatalf("unexpected rewritten body (prefix should be stripped): %s", rewritten)
	}
}

func TestOpenRouterAdminAddValidatesAndSavesAccount(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	openrouterBase, _ := url.Parse("https://openrouter.ai/api")
	validationCalled := false
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir, openrouterBase: openrouterBase},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewOpenRouterProvider(openrouterBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			validationCalled = true
			if req.Header.Get("Authorization") != "Bearer sk-or-valid" {
				t.Fatalf("validation auth = %q", req.Header.Get("Authorization"))
			}
			var body map[string]any
			payload, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(payload, &body)
			if body["model"] != "anthropic/claude-haiku-4.5" {
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

	req := httptest.NewRequest(http.MethodPost, "/admin/openrouter/add", strings.NewReader(`{"api_key":"sk-or-valid"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleOpenRouterAdd(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !validationCalled {
		t.Fatal("validation was not called")
	}
	entries, err := os.ReadDir(filepath.Join(poolDir, "openrouter"))
	if err != nil {
		t.Fatalf("read openrouter pool dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("saved %d OpenRouter files, want 1", len(entries))
	}
	if h.pool.countByType(AccountTypeOpenRouter) != 1 {
		t.Fatalf("pool OpenRouter count = %d, want 1", h.pool.countByType(AccountTypeOpenRouter))
	}
}

func TestOpenRouterAdminRejectsUnauthorizedKeyWithoutSaving(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	openrouterBase, _ := url.Parse("https://openrouter.ai/api")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir, openrouterBase: openrouterBase},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewOpenRouterProvider(openrouterBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader(`{"error":"bad key"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/openrouter/add", strings.NewReader(`{"api_key":"sk-or-bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleOpenRouterAdd(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(poolDir, "openrouter")); !os.IsNotExist(err) {
		t.Fatalf("openrouter pool dir should not exist after rejected key, err=%v", err)
	}
}
