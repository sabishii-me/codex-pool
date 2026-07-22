package main

import (
	"testing"
)

func TestAnthropicMessagesEngineNormalizesCompatibleProviders(t *testing.T) {
	providers := map[string]Provider{
		"claude":        &ClaudeProvider{},
		"deepseek":      &DeepSeekProvider{},
		"zai":           &ZAIProvider{},
		"minimax":       NewMinimaxProvider(nil),
		"openrouter":    NewOpenRouterProvider(nil),
		"qwen":          NewQwenProvider(nil),
		"kimi":          &KimiProvider{},
		"kimi-platform": NewKimiPlatformProvider(nil),
		"xiaomi":        NewXiaomiProvider(nil),
	}
	fixtures := []map[string]any{
		{
			"type": "message_start",
			"message": map[string]any{
				"model": "canonical-model",
				"usage": map[string]any{"input_tokens": 120, "cache_read_input_tokens": 20, "cache_creation_input_tokens": 10},
			},
		},
		{
			"type":  "message_delta",
			"usage": map[string]any{"output_tokens": 30, "reasoning_tokens": 5},
		},
		{
			"type":  "message",
			"model": "canonical-model",
			"usage": map[string]any{"input_tokens": 120, "cache_read_input_tokens": 20, "cache_creation_input_tokens": 10, "output_tokens": 30, "reasoning_tokens": 5},
		},
	}
	for providerName, provider := range providers {
		for index, fixture := range fixtures {
			want := anthropicMessagesEngine.ParseUsage(fixture)
			got := provider.ParseUsage(fixture)
			if want != nil {
				want.Timestamp = got.Timestamp
			}
			if got == nil || want == nil || *got != *want {
				t.Errorf("%s fixture %d usage=%#v, engine=%#v", providerName, index, got, want)
			}
		}
	}
}

func TestMixedAnthropicProvidersRetainTopLevelUsageFallback(t *testing.T) {
	fixtures := map[string]struct {
		provider Provider
		usage    map[string]any
	}{
		"kimi":          {provider: &KimiProvider{}, usage: map[string]any{"prompt_tokens": 40, "completion_tokens": 10, "cached_tokens": 5}},
		"kimi-platform": {provider: NewKimiPlatformProvider(nil), usage: map[string]any{"prompt_tokens": 40, "completion_tokens": 10, "cached_tokens": 5}},
		"xiaomi":        {provider: NewXiaomiProvider(nil), usage: map[string]any{"input_tokens": 40, "output_tokens": 10, "cache_read_input_tokens": 5}},
	}
	for name, fixture := range fixtures {
		usage := fixture.provider.ParseUsage(map[string]any{"model": "fallback-model", "usage": fixture.usage})
		if usage == nil || usage.InputTokens != 40 || usage.OutputTokens != 10 || usage.CachedInputTokens != 5 || usage.Model != "fallback-model" {
			t.Errorf("%s fallback usage=%#v", name, usage)
		}
	}
}
