package main

import "testing"

func TestOpenAIChatUsageNormalizesNestedCacheAndReasoning(t *testing.T) {
	fixture := map[string]any{
		"model": "nvidia/model",
		"usage": map[string]any{
			"prompt_tokens": float64(100), "completion_tokens": float64(25),
			"prompt_tokens_details":     map[string]any{"cached_tokens": float64(30)},
			"completion_tokens_details": map[string]any{"reasoning_tokens": float64(7)},
		},
	}
	usage := parseOpenAIChatUsage(fixture)
	if usage == nil {
		t.Fatal("usage is nil")
	}
	if usage.InputTokens != 100 || usage.CachedInputTokens != 30 || usage.OutputTokens != 25 || usage.ReasoningTokens != 7 || usage.BillableTokens != 95 || usage.Model != "nvidia/model" {
		t.Fatalf("unexpected usage: %+v", *usage)
	}
}

func TestNvidiaUsageUsesSharedOpenAIChatSemantics(t *testing.T) {
	provider := &NvidiaProvider{}
	usage := provider.ParseUsage(map[string]any{
		"model": "model", "usage": map[string]any{
			"prompt_tokens": float64(100), "completion_tokens": float64(25),
			"prompt_tokens_details":     map[string]any{"cached_tokens": float64(30)},
			"completion_tokens_details": map[string]any{"reasoning_tokens": float64(7)},
		},
	})
	if usage == nil || usage.CachedInputTokens != 30 || usage.ReasoningTokens != 7 || usage.BillableTokens != 95 {
		t.Fatalf("unexpected NVIDIA usage: %+v", usage)
	}
}
