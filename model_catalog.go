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

// poolModels is the hardcoded model catalog for plugin providers that cannot
// be expressed as declarative provider specs (openai-responses / custom
// endpoints). Standard providers (deepseek, minimax, zai, xiaomi, qwen,
// openrouter, nvidia, kimi-platform) get their model data exclusively from
// provider-specs/*.json - the single data source.
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
