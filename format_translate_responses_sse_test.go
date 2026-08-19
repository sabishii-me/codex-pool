package main

import "testing"

func TestAnthropicUsageMapFromResponsesOmitsUnreportedCacheKeys(t *testing.T) {
	// Upstream reported no cache fields: the translated Anthropic usage must
	// NOT include cache_read_input_tokens / cache_creation_input_tokens keys.
	// A downstream client (pi-ai / Claude style) must see "cache unavailable",
	// not a fabricated 0.
	usage := map[string]any{
		"input_tokens":  int64(1000),
		"output_tokens": int64(200),
	}
	out := anthropicUsageMapFromResponses(usage, true)
	if _, ok := out["cache_read_input_tokens"]; ok {
		t.Fatalf("cache_read_input_tokens must be omitted when unreported: %#v", out)
	}
	if _, ok := out["cache_creation_input_tokens"]; ok {
		t.Fatalf("cache_creation_input_tokens must be omitted when unreported: %#v", out)
	}
	if out["input_tokens"] != int64(1000) || out["output_tokens"] != int64(200) {
		t.Fatalf("unexpected projection %#v", out)
	}
}

func TestAnthropicUsageMapFromResponsesKeepsReportedCache(t *testing.T) {
	// Upstream explicitly reported cache read=0 and write=5: both keys must be
	// present so downstream can distinguish known-zero from unavailable.
	usage := map[string]any{
		"input_tokens":                int64(1000),
		"output_tokens":               int64(200),
		"cache_read_input_tokens":     int64(0),
		"cache_creation_input_tokens": int64(5),
	}
	out := anthropicUsageMapFromResponses(usage, true)
	if v, ok := out["cache_read_input_tokens"]; !ok || v != int64(0) {
		t.Fatalf("cache_read_input_tokens=%v ok=%v, want 0 present", v, ok)
	}
	if v, ok := out["cache_creation_input_tokens"]; !ok || v != int64(5) {
		t.Fatalf("cache_creation_input_tokens=%v ok=%v, want 5 present", v, ok)
	}
}

func TestAnthropicUsageMapFromResponsesInputTokensDetailsAlias(t *testing.T) {
	// Some Responses providers report cache under input_tokens_details.cached_tokens.
	usage := map[string]any{
		"input_tokens":  int64(1000),
		"output_tokens": int64(200),
		"input_tokens_details": map[string]any{
			"cached_tokens": int64(80),
		},
	}
	out := anthropicUsageMapFromResponses(usage, true)
	if v, ok := out["cache_read_input_tokens"]; !ok || v != int64(80) {
		t.Fatalf("cache_read_input_tokens=%v ok=%v, want 80 from details", v, ok)
	}
	if out["input_tokens"] != int64(920) {
		t.Fatalf("input_tokens=%v, want 920 (exclusive cache)", out["input_tokens"])
	}
}

func TestAnthropicUsageFromResponsesPresenceFlags(t *testing.T) {
	absent := anthropicUsageFromResponses(map[string]any{"input_tokens": int64(10)})
	if absent.CacheReadReported || absent.CacheCreationReported {
		t.Fatalf("absent presence flags must be false: %+v", absent)
	}
	present := anthropicUsageFromResponses(map[string]any{
		"input_tokens":            int64(10),
		"cache_read_input_tokens": int64(3),
	})
	if !present.CacheReadReported || present.CacheCreationReported {
		t.Fatalf("present flags=%+v", present)
	}
}
