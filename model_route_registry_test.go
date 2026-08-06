package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"testing"
)

func modelRouteTestRegistry(base *url.URL) *ProviderRegistry {
	return NewProviderRegistry(
		NewCodexProvider(base, base, base), NewGeminiProvider(base, base),
		NewAntigravityProvider(base, base), NewKimiProvider(base), NewGrokProvider(base),
		NewDeepSeekProvider(base), NewNvidiaProvider(base),
	)
}

func TestModelRouteRegistryCentralizesProviderPrecedenceAndPolicies(t *testing.T) {
	base, _ := url.Parse("https://routes.example.test")
	routes := NewModelRouteRegistry(modelRouteTestRegistry(base))
	cases := []struct {
		name, path, model string
		provider          ProviderID
		canonical         string
		policy            ModelBodyPolicy
	}{
		{name: "declarative alias", path: "/v1/messages", model: "deepseek", provider: AccountTypeDeepSeek, canonical: "deepseek-v4-pro", policy: ModelBodyRewriteNative},
		{name: "declarative prefix", path: "/v1/messages", model: "nvidia/meta/model", provider: AccountTypeNvidia, canonical: "meta/model", policy: ModelBodyRewriteNative},
		{name: "antigravity", path: "/v1/responses", model: "antigravity/gemini-3-flash", provider: AccountTypeAntigravity, canonical: "gemini-3-flash", policy: ModelBodyCustomAntigravity},
		{name: "kimi coding", path: "/v1/messages", model: "kimi-for-coding", provider: AccountTypeKimi, canonical: "kimi-for-coding", policy: ModelBodyRewriteNative},
		{name: "grok", path: "/v1/responses", model: "grok-composer", provider: AccountTypeGrok, canonical: "grok-composer-2.5-fast", policy: ModelBodySanitizeGrok},
		{name: "codex", path: "/v1/messages", model: "gpt-5.5", provider: AccountTypeCodex, canonical: "gpt-5.5", policy: ModelBodyRewriteNative},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route, ok := routes.Resolve(tc.path, tc.model)
			if !ok || route.Provider.Type() != tc.provider || route.CanonicalModel != tc.canonical || route.BodyPolicy != tc.policy {
				t.Fatalf("route=%#v ok=%v", route, ok)
			}
		})
	}
}

func TestResolvedModelRouteAppliesOneBodyPolicy(t *testing.T) {
	base, _ := url.Parse("https://routes.example.test")
	routes := NewModelRouteRegistry(modelRouteTestRegistry(base))
	body := []byte(`{"model":"deepseek","metadata":{"keep":true}}`)
	native, _ := routes.Resolve("/v1/messages", "deepseek")
	var rewritten map[string]any
	if got := native.RewriteBody(body); json.Unmarshal(got, &rewritten) != nil || rewritten["model"] != "deepseek-v4-pro" || rewritten["metadata"].(map[string]any)["keep"] != true {
		t.Fatalf("native rewrite=%s", got)
	}
	grok, _ := routes.Resolve("/v1/responses", "grok-composer")
	got := grok.RewriteBody([]byte(`{"model":"grok-composer","metadata":{"remove":true},"input":"keep"}`))
	if bytes.Contains(got, []byte("metadata")) || !bytes.Contains(got, []byte(`"model":"grok-composer-2.5-fast"`)) {
		t.Fatalf("Grok policy output=%s", got)
	}
	if !grok.RequiresWholeBody() {
		t.Fatal("Grok route did not declare whole-body requirement")
	}
}

func TestModelRouteRegistryObservesAtomicProviderReload(t *testing.T) {
	base, _ := url.Parse("https://routes.example.test")
	providers := NewProviderRegistry(NewCodexProvider(base, base, base), NewGeminiProvider(base, base))
	routes := NewModelRouteRegistry(providers)
	if _, ok := routes.Resolve("/v1/messages", "runtime"); ok {
		t.Fatal("runtime route existed before reload")
	}
	spec := validProviderSpec()
	spec.Models[0].Aliases = []string{"runtime"}
	if err := providers.ReplaceDeclarative([]ProviderSpec{spec}); err != nil {
		t.Fatal(err)
	}
	route, ok := routes.Resolve("/v1/messages", "runtime")
	if !ok || route.Provider.Type() != spec.ID || route.CanonicalModel != "example-model" {
		t.Fatalf("reloaded route=%#v ok=%v", route, ok)
	}
}
