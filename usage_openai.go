package main

import "time"

// parseOpenAIChatUsage normalizes Chat Completions usage from both complete
// responses and final streaming chunks. It supports OpenAI's nested token
// detail fields while retaining compatibility with providers that expose
// cached/reasoning counts at the usage top level.
func parseOpenAIChatUsage(obj map[string]any) *RequestUsage {
	usageMap, ok := obj["usage"].(map[string]any)
	if !ok || usageMap == nil {
		return nil
	}
	usage := &RequestUsage{Timestamp: time.Now()}
	usage.InputTokens = readInt64(usageMap, "prompt_tokens")
	if usage.InputTokens == 0 {
		usage.InputTokens = readInt64(usageMap, "input_tokens")
	}
	usage.OutputTokens = readInt64(usageMap, "completion_tokens")
	if usage.OutputTokens == 0 {
		usage.OutputTokens = readInt64(usageMap, "output_tokens")
	}
	usage.CachedInputTokens = readInt64(usageMap, "cached_tokens")
	if details, ok := usageMap["prompt_tokens_details"].(map[string]any); ok && usage.CachedInputTokens == 0 {
		usage.CachedInputTokens = readInt64(details, "cached_tokens")
	}
	usage.ReasoningTokens = readInt64(usageMap, "reasoning_tokens")
	if details, ok := usageMap["completion_tokens_details"].(map[string]any); ok && usage.ReasoningTokens == 0 {
		usage.ReasoningTokens = readInt64(details, "reasoning_tokens")
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.ReasoningTokens == 0 {
		return nil
	}
	usage.BillableTokens = clampNonNegative(usage.InputTokens - usage.CachedInputTokens + usage.OutputTokens)
	usage.Model, _ = obj["model"].(string)
	return usage
}
