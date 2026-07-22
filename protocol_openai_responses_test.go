package main

import "testing"

func TestOpenAIResponsesEngineNormalizesTopLevelAndNestedUsage(t *testing.T) {
	usage := map[string]any{
		"input_tokens": 120, "output_tokens": 40,
		"input_tokens_details":  map[string]any{"cached_tokens": 30},
		"output_tokens_details": map[string]any{"reasoning_tokens": 7},
	}
	fixtures := []map[string]any{
		{"model": "top-level-model", "prompt_cache_key": "cache-top", "usage": usage},
		{"type": "response.completed", "prompt_cache_key": "cache-event", "response": map[string]any{"model": "nested-model", "usage": usage}},
	}
	for index, fixture := range fixtures {
		got := openAIResponsesEngine.ParseUsage(fixture)
		if got == nil || got.InputTokens != 120 || got.CachedInputTokens != 30 || got.OutputTokens != 40 || got.ReasoningTokens != 7 || got.BillableTokens != 130 {
			t.Fatalf("fixture %d usage=%#v", index, got)
		}
	}
	if got := openAIResponsesEngine.ParseUsage(fixtures[0]); got.Model != "top-level-model" || got.PromptCacheKey != "cache-top" {
		t.Fatalf("top-level attribution=%#v", got)
	}
	if got := openAIResponsesEngine.ParseUsage(fixtures[1]); got.Model != "nested-model" || got.PromptCacheKey != "cache-event" {
		t.Fatalf("nested attribution=%#v", got)
	}
}

func TestOpenAIResponsesEngineOptionsPreserveProviderSemantics(t *testing.T) {
	grok := grokResponsesEngine.ParseUsage(map[string]any{
		"model": "grok-model",
		"usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 25, "cached_tokens": 30},
	})
	if grok == nil || grok.InputTokens != 100 || grok.OutputTokens != 25 || grok.CachedInputTokens != 30 || grok.BillableTokens != 95 || grok.Model != "grok-model" {
		t.Fatalf("Grok alias usage=%#v", grok)
	}

	sampled := sampledResponsesEngine.ParseUsage(map[string]any{
		"model": "sampled-model", "prompt_cache_key": "sampled-cache",
		"usage": map[string]any{
			"input_tokens": 100, "cache_read_input_tokens": 20, "cache_creation_input_tokens": 10,
			"output_tokens": 25, "reasoning_output_tokens": 7, "billable_tokens": 91,
		},
	})
	if sampled == nil || sampled.CacheCreationTokens != 10 || sampled.ReasoningTokens != 7 || sampled.BillableTokens != 91 || sampled.PromptCacheKey != "sampled-cache" {
		t.Fatalf("sampled usage=%#v", sampled)
	}
}

func TestResponsesCompatibleProvidersDelegateToEngine(t *testing.T) {
	fixture := map[string]any{"type": "response.completed", "response": map[string]any{
		"model": "responses-model",
		"usage": map[string]any{"input_tokens": 120, "output_tokens": 40, "input_tokens_details": map[string]any{"cached_tokens": 30}, "output_tokens_details": map[string]any{"reasoning_tokens": 7}},
	}}
	for name, provider := range map[string]Provider{"codex": &CodexProvider{}, "grok": &GrokProvider{}} {
		got := provider.ParseUsage(fixture)
		if got == nil || got.InputTokens != 120 || got.CachedInputTokens != 30 || got.OutputTokens != 40 || got.ReasoningTokens != 7 || got.BillableTokens != 130 || got.Model != "responses-model" {
			t.Errorf("%s usage=%#v", name, got)
		}
	}
}

func TestCodexKeepsTokenCountAsCustomFallback(t *testing.T) {
	provider := &CodexProvider{}
	got := provider.ParseUsage(map[string]any{"info": map[string]any{"model": "token-count-model", "last_token_usage": map[string]any{"input_tokens": 20, "cached_input_tokens": 5, "output_tokens": 3, "reasoning_output_tokens": 2}}})
	if got == nil || got.InputTokens != 20 || got.CachedInputTokens != 5 || got.OutputTokens != 3 || got.ReasoningTokens != 2 || got.BillableTokens != 18 || got.Model != "token-count-model" {
		t.Fatalf("token_count usage=%#v", got)
	}
}
