package main

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
)

func TestWebSocketFrameUsesCentralModelRoute(t *testing.T) {
	base, _ := url.Parse("https://ws-routes.example.test")
	codex := NewCodexProvider(base, base, base)
	handler := &proxyHandler{registry: NewProviderRegistry(codex, NewClaudeProvider(base), NewGeminiProvider(base, base), NewDeepSeekProvider(base)), aliases: newModelAliases(nil)}

	frame := applyModelAliasToJSONFrame(handler, "test", []byte(`{"type":"response.create","model":"gpt-5.6"}`))
	routed, err := applyModelRouteToWebSocketFrame(handler, codex, frame)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(routed, []byte(`"model":"gpt-5.6-sol"`)) {
		t.Fatalf("canonical frame=%s", routed)
	}
}

func TestWebSocketFrameRejectsProviderSwitchAfterUpgrade(t *testing.T) {
	base, _ := url.Parse("https://ws-routes.example.test")
	codex := NewCodexProvider(base, base, base)
	handler := &proxyHandler{registry: NewProviderRegistry(codex, NewClaudeProvider(base), NewGeminiProvider(base, base), NewDeepSeekProvider(base))}

	frame := []byte(`{"type":"response.create","model":"deepseek"}`)
	_, err := applyModelRouteToWebSocketFrame(handler, codex, frame)
	if err == nil || !strings.Contains(err.Error(), "routes to deepseek") {
		t.Fatalf("provider switch error=%v", err)
	}
}
