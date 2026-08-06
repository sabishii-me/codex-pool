package main

import "strings"

type ModelRoute struct {
	ProviderID    ProviderID
	ID            string
	DisplayName   string
	Description   string
	Aliases       []string
	ContextWindow int
	MaxTokens     int
	Reasoning     bool
	Input         []string
	Cost          *piModelCost
}

// poolModel is retained while catalog call sites migrate to ModelRoute.
// Deprecated: use ModelRoute.
type poolModel = ModelRoute

var poolModels = []ModelRoute{
	{ProviderID: AccountTypeCodex, ID: "gpt-5.6-sol", DisplayName: "GPT-5.6-Sol", Description: "Latest frontier agentic coding model.", ContextWindow: 272000, MaxTokens: 128000, Reasoning: true, Input: []string{"text", "image"}, Aliases: []string{"gpt-5.6"}},
	{ProviderID: AccountTypeCodex, ID: "gpt-5.6-terra", DisplayName: "GPT-5.6-Terra", Description: "Balanced agentic coding model for everyday work.", ContextWindow: 272000, MaxTokens: 128000, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeCodex, ID: "gpt-5.6-luna", DisplayName: "GPT-5.6-Luna", Description: "Fast and affordable agentic coding model.", ContextWindow: 272000, MaxTokens: 128000, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeCodex, ID: "gpt-5.5", DisplayName: "GPT-5.5", Description: "Frontier model for complex coding, research, and real-world work.", ContextWindow: 272000, MaxTokens: 128000, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeCodex, ID: "gpt-5.4", DisplayName: "GPT-5.4", Description: "Strong model for everyday coding.", ContextWindow: 272000, MaxTokens: 128000, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeCodex, ID: "gpt-5.4-mini", DisplayName: "GPT-5.4-Mini", Description: "Small, fast, and cost-efficient model for simpler coding tasks.", ContextWindow: 272000, MaxTokens: 128000, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeCodex, ID: "gpt-5.3-codex-spark", DisplayName: "GPT-5.3-Codex-Spark", Description: "Ultra-fast coding model.", ContextWindow: 128000, MaxTokens: 128000, Reasoning: true, Input: []string{"text", "image"}},


	{ProviderID: AccountTypeKimi, ID: "kimi-for-coding", DisplayName: "kimi-for-coding", ContextWindow: 262144, MaxTokens: 32768, Reasoning: true, Input: []string{"text", "image"}, Aliases: []string{"kimi", "k2p5", "kimi-k2-thinking"}},
	{ProviderID: AccountTypeKimi, ID: "kimi-for-coding-highspeed", DisplayName: "kimi-for-coding-highspeed", ContextWindow: 262144, MaxTokens: 32768, Reasoning: true, Input: []string{"text", "image"}},

	// Kimi Open Platform is a separate pay-as-you-go product from the Kimi
	// Coding Plan. Keep its real model IDs and limits aligned with the official
	// Open Platform catalog; "kimi-for-coding" belongs only to AccountTypeKimi.
	{ProviderID: AccountTypeKimiPlatform, ID: "kimi-k3", DisplayName: "Kimi K3", Description: "Latest Moonshot/Kimi Open Platform frontier model.", ContextWindow: 1048576, MaxTokens: 131072, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeKimiPlatform, ID: "kimi-k3[1m]", DisplayName: "Kimi K3 (Anthropic 1M alias)", Description: "Kimi K3 alias documented for Anthropic-compatible clients.", ContextWindow: 1048576, MaxTokens: 131072, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeKimiPlatform, ID: "kimi-k2.7-code", DisplayName: "Kimi K2.7-Code", Description: "Open Platform coding model.", ContextWindow: 262144, MaxTokens: 32768, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeKimiPlatform, ID: "kimi-k2.7-code-highspeed", DisplayName: "Kimi K2.7-Code Highspeed", Description: "Open Platform coding model, faster inference.", ContextWindow: 262144, MaxTokens: 32768, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeKimiPlatform, ID: "kimi-k2.6", DisplayName: "Kimi K2.6", Description: "Open Platform general-purpose model.", ContextWindow: 262144, MaxTokens: 32768, Reasoning: true, Input: []string{"text", "image"}},
	{ProviderID: AccountTypeKimiPlatform, ID: "kimi-k2.5", DisplayName: "Kimi K2.5", Description: "Open Platform legacy model.", ContextWindow: 262144, MaxTokens: 32768, Reasoning: true, Input: []string{"text", "image"}},

	{ProviderID: AccountTypeMinimax, ID: "MiniMax-M3", DisplayName: "MiniMax-M3", ContextWindow: 1000000, MaxTokens: 131072, Reasoning: true, Input: []string{"text", "image"}, Aliases: []string{"minimax", "minimax-m3"}},
	{ProviderID: AccountTypeMinimax, ID: "MiniMax-M2.7", DisplayName: "MiniMax-M2.7", ContextWindow: 204800, MaxTokens: 131072, Reasoning: true, Input: []string{"text", "image"}, Cost: &piModelCost{Input: 0.3, Output: 1.2, CacheRead: 0.06, CacheWrite: 0.375}},
	{ProviderID: AccountTypeMinimax, ID: "MiniMax-M2.7-highspeed", DisplayName: "MiniMax-M2.7-Highspeed", ContextWindow: 204800, MaxTokens: 131072, Reasoning: true, Input: []string{"text", "image"}, Cost: &piModelCost{Input: 0.6, Output: 2.4, CacheRead: 0.06, CacheWrite: 0.375}},

	{ProviderID: AccountTypeZAI, ID: "glm-5.2", DisplayName: "GLM-5.2", ContextWindow: 1000000, MaxTokens: 65536, Reasoning: true, Input: []string{"text"}},

	{ProviderID: AccountTypeXiaomi, ID: "mimo-v2.5-pro", DisplayName: "MiMo-V2.5-Pro", ContextWindow: 1000000, MaxTokens: 131072, Reasoning: true, Input: []string{"text"}, Aliases: []string{"mimo-v2.5-pro[1m]"}},

	{ProviderID: AccountTypeDeepSeek, ID: "deepseek-v4-flash", DisplayName: "DeepSeek-V4-Flash", Description: "Fast, cost-efficient DeepSeek model (non-thinking mode).", ContextWindow: 128000, MaxTokens: 32768, Reasoning: false, Input: []string{"text"}, Aliases: []string{"deepseek-flash"}},
	{ProviderID: AccountTypeDeepSeek, ID: "deepseek-v4-pro", DisplayName: "DeepSeek-V4-Pro", Description: "Frontier DeepSeek reasoning model.", ContextWindow: 128000, MaxTokens: 65536, Reasoning: true, Input: []string{"text"}, Aliases: []string{"deepseek-pro", "deepseek"}},

	{ProviderID: AccountTypeQwen, ID: "qwen3.6-plus", DisplayName: "Qwen3.6-Plus", Description: "Alibaba's flagship agentic coding model (Coding Plan).", ContextWindow: 1000000, MaxTokens: 65536, Reasoning: true, Input: []string{"text"}, Aliases: []string{"qwen", "qwen-coder", "qwen3-coder"}},

	// Aggregator catalogs are open-ended and cannot be represented honestly by
	// a made-up inventory row. Runtime-prefixed slugs still route through the
	// provider matchers, but the UI lists only models discovered or explicitly
	// declared by a provider contract.
}

func modelsForProvider(providerID ProviderID) []ModelRoute {
	var models []ModelRoute
	for _, model := range poolModels {
		if model.ProviderID == providerID {
			models = append(models, model)
		}
	}
	return models
}

func modelForProvider(providerID ProviderID, name string) (ModelRoute, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, model := range poolModels {
		if model.ProviderID != providerID {
			continue
		}
		if strings.EqualFold(model.ID, name) {
			return model, true
		}
		for _, alias := range model.Aliases {
			if strings.EqualFold(alias, name) {
				return model, true
			}
		}
	}
	return ModelRoute{}, false
}
