package main

import (
	"net/url"
	"testing"
)

func anthropicCompatibleProvidersForTest(t *testing.T) []Provider {
	t.Helper()
	base, err := url.Parse("https://anthropic-compatible.test")
	if err != nil {
		t.Fatal(err)
	}
	return []Provider{
		NewClaudeProvider(base), NewKimiProvider(base), NewKimiPlatformProvider(base),
		NewMinimaxProvider(base), NewZAIProvider(base), NewXiaomiProvider(base),
		NewDeepSeekProvider(base), NewQwenProvider(base), NewOpenRouterProvider(base),
	}
}

func TestAnthropicCompatibleProvidersShareStreamingUsageSemantics(t *testing.T) {
	start := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"model": "contract-model",
			"usage": map[string]any{
				"input_tokens": float64(120), "cache_read_input_tokens": float64(30),
				"cache_creation_input_tokens": float64(20),
			},
		},
	}
	delta := map[string]any{
		"type":  "message_delta",
		"usage": map[string]any{"output_tokens": float64(40), "reasoning_tokens": float64(7)},
	}
	for _, provider := range anthropicCompatibleProvidersForTest(t) {
		provider := provider
		t.Run(string(provider.Type()), func(t *testing.T) {
			var accumulator splitUsageAccumulator
			if completed := accumulator.add("message_start", provider.ParseUsage(start)); completed != nil {
				t.Fatal("message_start completed usage before terminal event")
			}
			usage := accumulator.add("message_delta", provider.ParseUsage(delta))
			assertNormalizedAnthropicUsage(t, usage)
		})
	}
}

func TestAnthropicCompatibleProvidersShareNonStreamingUsageSemantics(t *testing.T) {
	message := map[string]any{
		"id": "msg_contract", "type": "message", "model": "contract-model",
		"usage": map[string]any{
			"input_tokens": float64(120), "cache_read_input_tokens": float64(30),
			"cache_creation_input_tokens": float64(20), "output_tokens": float64(40),
			"output_tokens_details": map[string]any{"reasoning_tokens": float64(7)},
		},
	}
	for _, provider := range anthropicCompatibleProvidersForTest(t) {
		provider := provider
		t.Run(string(provider.Type()), func(t *testing.T) {
			assertNormalizedAnthropicUsage(t, provider.ParseUsage(message))
		})
	}
}

func TestGenericNonStreamingUsagePreservesCacheWriteAndReasoning(t *testing.T) {
	usage := parseRequestUsage(map[string]any{
		"model": "contract-model",
		"usage": map[string]any{
			"input_tokens": float64(120), "cache_read_input_tokens": float64(30),
			"cache_creation_input_tokens": float64(20), "output_tokens": float64(40),
			"output_tokens_details": map[string]any{"reasoning_tokens": float64(7)},
		},
	})
	assertNormalizedAnthropicUsage(t, usage)
}

func assertNormalizedAnthropicUsage(t *testing.T, usage *RequestUsage) {
	t.Helper()
	if usage == nil {
		t.Fatal("usage is nil")
	}
	if usage.InputTokens != 120 || usage.CachedInputTokens != 30 || usage.CacheCreationTokens != 20 || usage.OutputTokens != 40 || usage.ReasoningTokens != 7 {
		t.Fatalf("unexpected usage: %+v", *usage)
	}
	if usage.BillableTokens != 110 {
		t.Fatalf("billable tokens = %d, want 110", usage.BillableTokens)
	}
	if usage.Model != "contract-model" {
		t.Fatalf("model = %q, want contract-model", usage.Model)
	}
}
