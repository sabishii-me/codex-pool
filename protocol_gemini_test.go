package main

import "testing"

func TestGeminiUsageEngineNormalizesNativeDimensions(t *testing.T) {
	usage := geminiUsageEngine.ParseUsage(map[string]any{"usageMetadata": map[string]any{
		"promptTokenCount": 120, "cachedContentTokenCount": 30,
		"candidatesTokenCount": 40, "thoughtsTokenCount": 7, "totalTokenCount": 167,
	}})
	if usage == nil || usage.InputTokens != 120 || usage.CachedInputTokens != 30 || usage.OutputTokens != 40 || usage.ReasoningTokens != 7 || usage.BillableTokens != 130 {
		t.Fatalf("Gemini usage=%#v", usage)
	}
}

func TestGeminiUsageEngineMakesAntigravityEnvelopeExplicit(t *testing.T) {
	fixture := map[string]any{"response": map[string]any{"usageMetadata": map[string]any{
		"promptTokenCount": 120, "cachedContentTokenCount": 30,
		"candidatesTokenCount": 40, "thoughtsTokenCount": 7,
	}}}
	if usage := geminiUsageEngine.ParseUsage(fixture); usage != nil {
		t.Fatalf("plain Gemini unexpectedly unwrapped custom envelope: %#v", usage)
	}
	usage := antigravityGeminiUsageEngine.ParseUsage(fixture)
	if usage == nil || usage.InputTokens != 120 || usage.CachedInputTokens != 30 || usage.OutputTokens != 40 || usage.ReasoningTokens != 7 || usage.BillableTokens != 130 {
		t.Fatalf("Antigravity Gemini usage=%#v", usage)
	}
}

func TestGeminiUsageEnginePreservesReasoningOnlyPolicy(t *testing.T) {
	fixture := map[string]any{"usageMetadata": map[string]any{"thoughtsTokenCount": 7}}
	if usage := geminiUsageEngine.ParseUsage(fixture); usage != nil {
		t.Fatalf("plain Gemini reasoning-only usage=%#v, want nil", usage)
	}
	usage := antigravityGeminiUsageEngine.ParseUsage(fixture)
	if usage == nil || usage.ReasoningTokens != 7 {
		t.Fatalf("Antigravity reasoning-only usage=%#v", usage)
	}
}

func TestGeminiCompatibleProvidersDelegateWithDeclaredEnvelope(t *testing.T) {
	metadata := map[string]any{"promptTokenCount": 100, "cachedContentTokenCount": 20, "candidatesTokenCount": 25, "thoughtsTokenCount": 5}
	gemini := (&GeminiProvider{}).ParseUsage(map[string]any{"usageMetadata": metadata})
	antigravity := (&AntigravityProvider{}).ParseUsage(map[string]any{"response": map[string]any{"usageMetadata": metadata}})
	for name, usage := range map[string]*RequestUsage{"gemini": gemini, "antigravity": antigravity} {
		if usage == nil || usage.InputTokens != 100 || usage.CachedInputTokens != 20 || usage.OutputTokens != 25 || usage.ReasoningTokens != 5 || usage.BillableTokens != 105 {
			t.Errorf("%s usage=%#v", name, usage)
		}
	}
}
