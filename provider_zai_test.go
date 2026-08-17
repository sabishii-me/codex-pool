package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestIsZAIModelHandlesCodingPlanModels(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"glm-5.2", "GLM-5.2"} {
		if !isZAIModel(model) {
			t.Fatalf("expected %q to route to zai", model)
		}
		if got := zaiCanonicalModel(model); got != strings.ToLower(model) {
			t.Fatalf("unexpected canonical model for %q: %q", model, got)
		}
	}

	// The declarative spec (provider-specs/zai.json) is the single data source
	// and lists all 12 official GLM coding-plan models; older glm-4.5/4.6/5.x
	// rows now route to zai too.
	for _, model := range []string{"glm-4.5", "glm-4.5-air", "glm-4.6", "glm-4.7", "glm-5", "glm-5-turbo", "glm-5.1"} {
		if !isZAIModel(model) {
			t.Fatalf("expected %q to route to zai", model)
		}
	}

	// Models outside the official catalog must not route to zai.
	for _, model := range []string{"glm-4.4", "glm-3.5", "claude-sonnet-4-5"} {
		if isZAIModel(model) {
			t.Fatalf("did not expect %q to route to zai", model)
		}
	}
}

func TestZAIParseUsageMessageStart(t *testing.T) {
	t.Parallel()

	p := NewZAIProvider(nil)

	// Full message_start with input, cached, model
	ru := p.ParseUsage(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"model": "glm-5.2",
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
	if ru.Model != "glm-5.2" {
		t.Fatalf("Model = %q, want glm-5.2", ru.Model)
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

	// Non-Anthropic event type returns nil
	if got := p.ParseUsage(map[string]any{"type": "ping"}); got != nil {
		t.Fatal("expected nil for ping event")
	}
}

func TestZAIParseUsageMessageDelta(t *testing.T) {
	t.Parallel()

	p := NewZAIProvider(nil)

	ru := p.ParseUsage(map[string]any{
		"type":  "message_delta",
		"usage": map[string]any{"output_tokens": float64(88)},
	})
	if ru == nil {
		t.Fatal("expected non-nil usage")
	}
	if ru.OutputTokens != 88 {
		t.Fatalf("OutputTokens = %d, want 88", ru.OutputTokens)
	}
	if ru.BillableTokens != 88 {
		t.Fatalf("BillableTokens = %d, want 88", ru.BillableTokens)
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

func TestModelRouteOverrideZAIModelUsesZAIBase(t *testing.T) {
	t.Parallel()

	zaiBase, _ := url.Parse("https://api.z.ai/api/anthropic")
	handler := &proxyHandler{
		registry: NewProviderRegistry(
			&CodexProvider{},
			&GeminiProvider{},
			NewZAIProvider(zaiBase),
		),
	}

	provider, base, rewritten := handler.modelRouteOverride("/v1/messages", "GLM-5.2", []byte(`{"model":"GLM-5.2"}`))
	if provider == nil {
		t.Fatal("expected override provider")
	}
	if provider.Type() != AccountTypeZAI {
		t.Fatalf("expected zai provider, got %s", provider.Type())
	}
	if base == nil || base.String() != zaiBase.String() {
		t.Fatalf("expected zai base %s, got %v", zaiBase, base)
	}
	if string(rewritten) != `{"model":"glm-5.2"}` {
		t.Fatalf("unexpected rewritten body: %s", rewritten)
	}
}
