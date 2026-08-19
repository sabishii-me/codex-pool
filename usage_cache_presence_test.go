package main

import "testing"

func TestUsageParserCachePresenceFlags(t *testing.T) {
	// Anthropic Messages: absent cache field must leave CacheReadReported false.
	anthropic := parseAnthropicUsage(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"usage": map[string]any{"input_tokens": float64(100)},
		},
	})
	if anthropic == nil {
		t.Fatal("nil anthropic usage")
	}
	if anthropic.CacheReadReported {
		t.Fatal("anthropic absent cache must not be reported")
	}

	// Anthropic Messages: present cache field (even zero) must be reported.
	anthropicZero := parseAnthropicUsage(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"usage": map[string]any{"input_tokens": float64(100), "cache_read_input_tokens": float64(0)},
		},
	})
	if anthropicZero == nil || !anthropicZero.CacheReadReported {
		t.Fatalf("anthropic present-zero cache must be reported: %+v", anthropicZero)
	}

	// OpenAI Chat: absent cache leaves unreported.
	oai := openAIChatEngine.ParseUsage(map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(10),
		},
	})
	if oai == nil {
		t.Fatal("nil openai usage")
	}
	if oai.CacheReadReported {
		t.Fatal("openai absent cache must not be reported")
	}

	// OpenAI Chat: prompt_tokens_details.cached_tokens present (even zero) reported.
	oaiZero := openAIChatEngine.ParseUsage(map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(10),
			"prompt_tokens_details": map[string]any{
				"cached_tokens": float64(0),
			},
		},
	})
	if oaiZero == nil || !oaiZero.CacheReadReported {
		t.Fatalf("openai present-zero cache must be reported: %+v", oaiZero)
	}
}
