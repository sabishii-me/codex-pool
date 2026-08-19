package main

import "time"

// parseAnthropicUsage normalizes Anthropic Messages usage for both streaming
// events and complete non-streaming message objects. Compatible providers must
// share these semantics instead of each maintaining a subtly different parser.
func parseAnthropicUsage(obj map[string]any) *RequestUsage {
	if obj == nil {
		return nil
	}
	eventType, _ := obj["type"].(string)
	switch eventType {
	case "message_start":
		message, ok := obj["message"].(map[string]any)
		if !ok {
			return nil
		}
		usageMap, ok := message["usage"].(map[string]any)
		if !ok {
			return nil
		}
		usage := anthropicUsageFromMap(usageMap, false)
		if usage != nil {
			usage.Model, _ = message["model"].(string)
		}
		return usage
	case "message_delta":
		usageMap, ok := obj["usage"].(map[string]any)
		if !ok {
			return nil
		}
		return anthropicUsageFromMap(usageMap, true)
	case "message":
		usageMap, ok := obj["usage"].(map[string]any)
		if !ok {
			return nil
		}
		usage := anthropicUsageFromMap(usageMap, true)
		if usage != nil {
			usage.Model, _ = obj["model"].(string)
		}
		return usage
	default:
		return nil
	}
}

func anthropicUsageFromMap(usageMap map[string]any, includeOutput bool) *RequestUsage {
	if usageMap == nil {
		return nil
	}
	usage := &RequestUsage{Timestamp: time.Now()}
	usage.InputTokens = readInt64(usageMap, "input_tokens")
	usage.CachedInputTokens = readInt64(usageMap, "cache_read_input_tokens")
	usage.CacheCreationTokens = readInt64(usageMap, "cache_creation_input_tokens")
	if _, ok := usageMap["cache_read_input_tokens"]; ok {
		usage.CacheReadReported = true
	}
	if _, ok := usageMap["cache_creation_input_tokens"]; ok {
		usage.CacheCreationReported = true
	}
	if includeOutput {
		usage.OutputTokens = readInt64(usageMap, "output_tokens")
	}
	usage.ReasoningTokens = readInt64(usageMap, "reasoning_tokens")
	if usage.ReasoningTokens == 0 {
		usage.ReasoningTokens = readInt64(usageMap, "reasoning_output_tokens")
	}
	if details, ok := usageMap["output_tokens_details"].(map[string]any); ok && usage.ReasoningTokens == 0 {
		usage.ReasoningTokens = readInt64(details, "reasoning_tokens")
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.ReasoningTokens == 0 {
		return nil
	}
	usage.BillableTokens = clampNonNegative(
		usage.InputTokens - usage.CachedInputTokens - usage.CacheCreationTokens + usage.OutputTokens,
	)
	return usage
}
