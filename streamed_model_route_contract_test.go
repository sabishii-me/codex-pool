package main

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestStreamedModelRouteCoversEveryModelRoutedProvider(t *testing.T) {
	base, _ := url.Parse("https://streamed-route.test")
	registry := anthropicContractRegistry(base)
	if err := registry.AddProviders(NewGrokProvider(base), NewNvidiaProvider(base), NewAntigravityProvider(base, base)); err != nil {
		t.Fatal(err)
	}
	handler := &proxyHandler{cfg: &config{}, registry: registry, aliases: newModelAliases(nil)}
	fallback := registry.ForType(AccountTypeKimi)

	tests := []struct {
		name      string
		model     string
		wantType  AccountType
		canonical string
	}{
		{"antigravity", "antigravity/gemini-3-flash", AccountTypeAntigravity, "gemini-3-flash"},
		{"kimi", "kimi-for-coding", AccountTypeKimi, "kimi-for-coding"},
		{"kimi-platform", "kimi-platform/kimi-k3", AccountTypeKimiPlatform, "kimi-k3"},
		{"minimax", "minimax", AccountTypeMinimax, "MiniMax-M3"},
		{"zai", "glm-5.2", AccountTypeZAI, "glm-5.2"},
		{"xiaomi", "mimo-v2.5-pro", AccountTypeXiaomi, "mimo-v2.5-pro"},
		{"grok", "grok-composer", AccountTypeGrok, "grok-composer-2.5-fast"},
		{"deepseek", "deepseek", AccountTypeDeepSeek, "deepseek-v4-pro"},
		{"qwen", "qwen-coder", AccountTypeQwen, "qwen3.6-plus"},
		{"openrouter", "openrouter/anthropic/claude-haiku-4.5", AccountTypeOpenRouter, "anthropic/claude-haiku-4.5"},
		{"nvidia", "nvidia/meta/llama-3.3-70b-instruct", AccountTypeNvidia, "meta/llama-3.3-70b-instruct"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			padding := strings.Repeat("x", streamedModelRoutePeekBytes+1024)
			original := []byte(`{"model":"` + test.model + `","messages":[{"role":"user","content":"` + padding + `"}]}`)
			request := &http.Request{Method: http.MethodPost, URL: &url.URL{Path: "/v1/messages"}, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(original)), ContentLength: int64(len(original))}
			provider, routedBase, err := handler.applyStreamedModelRoute(request, fallback, base, "route-contract")
			if test.wantType == AccountTypeGrok || test.wantType == AccountTypeAntigravity {
				if err == nil || !strings.Contains(err.Error(), "requires full-body translation or sanitization") {
					t.Fatalf("large custom route error = %v, want explicit full-body rejection", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if provider.Type() != test.wantType || routedBase.String() != base.String() {
				t.Fatalf("route = %s %v, want %s %v", provider.Type(), routedBase, test.wantType, base)
			}
			forwarded, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			want := bytes.Replace(original, []byte(`"model":"`+test.model+`"`), []byte(`"model":"`+test.canonical+`"`), 1)
			if !bytes.Equal(forwarded, want) {
				t.Fatalf("streamed body changed outside canonical model rewrite: got %d bytes, want %d", len(forwarded), len(want))
			}
			if request.ContentLength != int64(len(want)) {
				t.Fatalf("content length = %d, want %d", request.ContentLength, len(want))
			}
		})
	}
}
