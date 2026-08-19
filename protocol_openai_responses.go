package main

import "time"

// OpenAIResponsesOptions declares provider differences without duplicating the
// Responses usage parser.
type OpenAIResponsesOptions struct {
	AllowChatAliases    bool
	IncludeCacheWrites  bool
	HonorBillableTokens bool
	AllowBillableOnly   bool
}

// OpenAIResponsesEngine owns usage normalization for complete Responses
// objects and response.completed events. It accepts usage either at the event
// root or inside the nested response object.
type OpenAIResponsesEngine struct {
	Options OpenAIResponsesOptions
}

func (engine OpenAIResponsesEngine) ParseUsage(event map[string]any) *RequestUsage {
	if event == nil {
		return nil
	}
	source := event
	usageMap, _ := event["usage"].(map[string]any)
	if usageMap == nil {
		if response, ok := event["response"].(map[string]any); ok {
			source = response
			usageMap, _ = response["usage"].(map[string]any)
		}
	}
	if usageMap == nil {
		return nil
	}

	usage := &RequestUsage{Timestamp: time.Now()}
	usage.InputTokens = readInt64(usageMap, "input_tokens")
	usage.OutputTokens = readInt64(usageMap, "output_tokens")
	if engine.Options.AllowChatAliases {
		if usage.InputTokens == 0 {
			usage.InputTokens = readInt64(usageMap, "prompt_tokens")
		}
		if usage.OutputTokens == 0 {
			usage.OutputTokens = readInt64(usageMap, "completion_tokens")
		}
	}

	usage.CachedInputTokens = readInt64(usageMap, "cached_input_tokens")
	if _, ok := usageMap["cached_input_tokens"]; ok {
		usage.CacheReadReported = true
	}
	if usage.CachedInputTokens == 0 && !usage.CacheReadReported {
		if _, ok := usageMap["cache_read_input_tokens"]; ok {
			usage.CachedInputTokens = readInt64(usageMap, "cache_read_input_tokens")
			usage.CacheReadReported = true
		}
	}
	if engine.Options.AllowChatAliases && usage.CachedInputTokens == 0 && !usage.CacheReadReported {
		if _, ok := usageMap["cached_tokens"]; ok {
			usage.CachedInputTokens = readInt64(usageMap, "cached_tokens")
			usage.CacheReadReported = true
		}
	}
	if details, ok := usageMap["input_tokens_details"].(map[string]any); ok && usage.CachedInputTokens == 0 && !usage.CacheReadReported {
		if _, present := details["cached_tokens"]; present {
			usage.CachedInputTokens = readInt64(details, "cached_tokens")
			usage.CacheReadReported = true
		}
	}
	if engine.Options.IncludeCacheWrites {
		usage.CacheCreationTokens = readInt64(usageMap, "cache_creation_input_tokens")
		if _, ok := usageMap["cache_creation_input_tokens"]; ok {
			usage.CacheCreationReported = true
		}
	}

	usage.ReasoningTokens = readInt64(usageMap, "reasoning_output_tokens")
	if usage.ReasoningTokens == 0 {
		usage.ReasoningTokens = readInt64(usageMap, "reasoning_tokens")
	}
	if details, ok := usageMap["output_tokens_details"].(map[string]any); ok && usage.ReasoningTokens == 0 {
		usage.ReasoningTokens = readInt64(details, "reasoning_tokens")
	}

	if engine.Options.HonorBillableTokens {
		usage.BillableTokens = readInt64(usageMap, "billable_tokens")
	}
	if usage.BillableTokens == 0 {
		usage.BillableTokens = clampNonNegative(usage.InputTokens - usage.CachedInputTokens - usage.CacheCreationTokens + usage.OutputTokens)
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && (!engine.Options.AllowBillableOnly || usage.BillableTokens == 0) {
		return nil
	}

	usage.PromptCacheKey, _ = event["prompt_cache_key"].(string)
	if usage.PromptCacheKey == "" {
		usage.PromptCacheKey, _ = source["prompt_cache_key"].(string)
	}
	usage.Model, _ = source["model"].(string)
	if usage.Model == "" {
		usage.Model, _ = event["model"].(string)
	}
	return usage
}

var openAIResponsesEngine = OpenAIResponsesEngine{}
var grokResponsesEngine = OpenAIResponsesEngine{Options: OpenAIResponsesOptions{AllowChatAliases: true}}
var sampledResponsesEngine = OpenAIResponsesEngine{Options: OpenAIResponsesOptions{
	IncludeCacheWrites: true, HonorBillableTokens: true, AllowBillableOnly: true,
}}
