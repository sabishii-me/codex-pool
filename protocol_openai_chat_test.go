package main

import "testing"

func TestOpenAIChatEngineMakesCacheBillingPolicyExplicit(t *testing.T) {
	fixture := map[string]any{
		"model": "compatible-model",
		"usage": map[string]any{
			"prompt_tokens": 100, "completion_tokens": 25,
			"prompt_tokens_details":     map[string]any{"cached_tokens": 30},
			"completion_tokens_details": map[string]any{"reasoning_tokens": 7},
		},
	}
	excluding := openAIChatEngine.ParseUsage(fixture)
	including := openAIChatLegacyKimiEngine.ParseUsage(fixture)
	if excluding == nil || including == nil {
		t.Fatal("engine usage is nil")
	}
	if excluding.BillableTokens != 95 {
		t.Fatalf("cache-excluding billable=%d, want 95", excluding.BillableTokens)
	}
	if including.BillableTokens != 125 {
		t.Fatalf("cache-including billable=%d, want 125", including.BillableTokens)
	}
	for _, usage := range []*RequestUsage{excluding, including} {
		if usage.InputTokens != 100 || usage.CachedInputTokens != 30 || usage.OutputTokens != 25 || usage.ReasoningTokens != 7 || usage.Model != "compatible-model" {
			t.Fatalf("normalized dimensions changed: %#v", usage)
		}
	}
}

func TestOpenAIChatCompatibleProvidersDelegateWithDeclaredPolicy(t *testing.T) {
	fixture := map[string]any{
		"model": "compatible-model",
		"usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 25, "cached_tokens": 30},
	}
	cases := []struct {
		name     string
		provider Provider
		billable int64
	}{
		{name: "nvidia", provider: NewNvidiaProvider(nil), billable: 95},
		{name: "kimi", provider: &KimiProvider{}, billable: 125},
		{name: "kimi-platform", provider: NewKimiPlatformProvider(nil), billable: 125},
	}
	for _, tc := range cases {
		usage := tc.provider.ParseUsage(fixture)
		if usage == nil || usage.BillableTokens != tc.billable || usage.CachedInputTokens != 30 {
			t.Errorf("%s usage=%#v, want billable=%d", tc.name, usage, tc.billable)
		}
	}
}
