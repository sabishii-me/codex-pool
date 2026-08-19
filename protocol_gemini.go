package main

import "time"

// GeminiUsageOptions declares envelope and completion differences among native
// Gemini-compatible transports.
type GeminiUsageOptions struct {
	UnwrapResponse     bool
	AllowReasoningOnly bool
}

// GeminiUsageEngine owns normalization of usageMetadata token dimensions.
type GeminiUsageEngine struct {
	Options GeminiUsageOptions
}

func (engine GeminiUsageEngine) ParseUsage(event map[string]any) *RequestUsage {
	if event == nil {
		return nil
	}
	source := event
	if engine.Options.UnwrapResponse {
		if response, ok := event["response"].(map[string]any); ok {
			source = response
		}
	}
	metadata, ok := source["usageMetadata"].(map[string]any)
	if !ok || metadata == nil {
		return nil
	}
	usage := &RequestUsage{
		Timestamp:         time.Now(),
		InputTokens:       readInt64(metadata, "promptTokenCount"),
		CachedInputTokens: readInt64(metadata, "cachedContentTokenCount"),
		OutputTokens:      readInt64(metadata, "candidatesTokenCount"),
		ReasoningTokens:   readInt64(metadata, "thoughtsTokenCount"),
	}
	if _, ok := metadata["cachedContentTokenCount"]; ok {
		usage.CacheReadReported = true
	}
	usage.BillableTokens = clampNonNegative(usage.InputTokens - usage.CachedInputTokens + usage.OutputTokens)
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && (!engine.Options.AllowReasoningOnly || usage.ReasoningTokens == 0) {
		return nil
	}
	return usage
}

var geminiUsageEngine GeminiUsageEngine
var antigravityGeminiUsageEngine = GeminiUsageEngine{Options: GeminiUsageOptions{UnwrapResponse: true, AllowReasoningOnly: true}}
