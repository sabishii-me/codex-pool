package main

import (
	"encoding/json"
	"strings"
)

type piModelsConfig struct {
	Providers map[string]piProviderConfig `json:"providers"`
}

type piProviderConfig struct {
	BaseURL string          `json:"baseUrl,omitempty"`
	APIKey  string          `json:"apiKey,omitempty"`
	API     string          `json:"api,omitempty"`
	Models  []piModelConfig `json:"models,omitempty"`
}

type piModelConfig struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	Reasoning        *bool             `json:"reasoning,omitempty"`
	ThinkingLevelMap map[string]string `json:"thinkingLevelMap,omitempty"`
	Input            []string          `json:"input,omitempty"`
	ContextWindow    int               `json:"contextWindow,omitempty"`
	MaxTokens        int               `json:"maxTokens,omitempty"`
	Cost             *piModelCost      `json:"cost,omitempty"`
	Compat           *piModelCompat    `json:"compat,omitempty"`
}

type piModelCompat struct {
	ForceAdaptiveThinking bool `json:"forceAdaptiveThinking,omitempty"`
}

type piModelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

func generatePiModelsJSON(publicURL, codexAPIKey, anthropicAPIKey string, pricings ...*PricingData) ([]byte, error) {
	var pricing *PricingData
	if len(pricings) > 0 {
		pricing = pricings[0]
	}
	baseURL := strings.TrimRight(strings.TrimSpace(publicURL), "/")
	cfg := piModelsConfig{
		Providers: map[string]piProviderConfig{
			"codex": {
				BaseURL: baseURL + "/backend-api",
				APIKey:  codexAPIKey,
				API:     "openai-codex-responses",
				Models:  piModelsForProvider(AccountTypeCodex, pricing),
			},
			"antigravity": {
				BaseURL: baseURL,
				APIKey:  codexAPIKey,
				API:     "openai-completions",
				Models:  antigravityPiModels(),
			},
			"kimi": {
				BaseURL: baseURL,
				APIKey:  anthropicAPIKey,
				API:     "anthropic-messages",
				Models:  piModelsForProvider(AccountTypeKimi, pricing),
			},
			"kimi-platform": {
				BaseURL: baseURL,
				APIKey:  anthropicAPIKey,
				API:     "anthropic-messages",
				Models:  piModelsForProvider(AccountTypeKimiPlatform, pricing),
			},
			"minimax": {
				BaseURL: baseURL,
				APIKey:  anthropicAPIKey,
				API:     "anthropic-messages",
				Models:  piModelsForProvider(AccountTypeMinimax, pricing),
			},
			"zai": {
				BaseURL: baseURL,
				APIKey:  anthropicAPIKey,
				API:     "anthropic-messages",
				Models:  piModelsForProvider(AccountTypeZAI, pricing),
			},
			"xiaomi": {
				BaseURL: baseURL,
				APIKey:  anthropicAPIKey,
				API:     "anthropic-messages",
				Models:  piModelsForProvider(AccountTypeXiaomi, pricing),
			},
			"grok": {
				BaseURL: baseURL,
				APIKey:  codexAPIKey,
				API:     "openai-responses",
				Models:  grokPiModels(),
			},
			"deepseek": {
				BaseURL: baseURL,
				APIKey:  anthropicAPIKey,
				API:     "anthropic-messages",
				Models:  piModelsForProvider(AccountTypeDeepSeek, pricing),
			},
			"qwen": {
				BaseURL: baseURL,
				APIKey:  anthropicAPIKey,
				API:     "anthropic-messages",
				Models:  piModelsForProvider(AccountTypeQwen, pricing),
			},
			"openrouter": {
				BaseURL: baseURL,
				APIKey:  anthropicAPIKey,
				API:     "anthropic-messages",
				Models:  piModelsForProvider(AccountTypeOpenRouter, pricing),
			},
			"nvidia": {
				BaseURL: baseURL,
				APIKey:  codexAPIKey,
				API:     "openai-completions",
				Models:  piModelsForProvider(AccountTypeNvidia, pricing),
			},
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func antigravityPiModels() []piModelConfig {
	models := antigravityModels.Models(nil)
	result := make([]piModelConfig, 0, len(models))
	for _, model := range models {
		input := []string{"text"}
		if model.SupportsImages {
			input = append(input, "image")
		}
		result = append(result, piModelConfig{ID: "antigravity/" + model.ID, Name: model.DisplayName, Reasoning: boolPtr(model.SupportsThinking), Input: input, ContextWindow: model.MaxTokens, MaxTokens: model.MaxOutputTokens})
	}
	return result
}

func grokPiModels() []piModelConfig {
	models := make([]piModelConfig, 0, len(grokModelCatalog))
	for _, model := range grokModelCatalog {
		models = append(models, piTextModel(model.ID, model.Name, model.Reasoning, model.ContextWindow, model.MaxTokens))
	}
	return models
}

func piTextModel(id, name string, reasoning bool, contextWindow, maxTokens int) piModelConfig {
	return piModelConfig{
		ID:            id,
		Name:          name,
		Reasoning:     boolPtr(reasoning),
		Input:         []string{"text", "image"},
		ContextWindow: contextWindow,
		MaxTokens:     maxTokens,
	}
}

func piCodexModel(id, name string, contextWindow, maxTokens int) piModelConfig {
	model := piTextModel(id, name, true, contextWindow, maxTokens)
	if id == "gpt-5.6" || strings.HasPrefix(id, "gpt-5.6-") {
		model.ThinkingLevelMap = map[string]string{"xhigh": "xhigh", "max": "max"}
	}
	return model
}


func boolPtr(v bool) *bool {
	return &v
}

func piModelsForProvider(accountType AccountType, pricings ...*PricingData) []piModelConfig {
	var pricing *PricingData
	if len(pricings) > 0 {
		pricing = pricings[0]
	}
	models := modelsForProvider(accountType)
	result := make([]piModelConfig, 0, len(models))
	for _, model := range models {
		config := piModelConfig{
			ID:            model.ID,
			Name:          model.DisplayName,
			Reasoning:     boolPtr(model.Reasoning),
			Input:         append([]string(nil), model.Input...),
			ContextWindow: model.ContextWindow,
			MaxTokens:     model.MaxTokens,
			Cost:          piCostForModel(pricing, model),
		}
		if accountType == AccountTypeCodex && strings.HasPrefix(model.ID, "gpt-5.6-") {
			config.ThinkingLevelMap = map[string]string{"xhigh": "xhigh", "max": "max"}
		}
		result = append(result, config)
	}
	return result
}

func piCostForModel(pricing *PricingData, model ModelRoute) *piModelCost {
	sheet := priceSheetForModel(pricing, model)
	if sheet.Status != "known" || sheet.Rates == nil {
		return nil
	}
	cacheRead, cacheWrite := 0.0, 0.0
	if sheet.Rates.CacheRead != nil {
		cacheRead = *sheet.Rates.CacheRead
	}
	if sheet.Rates.CacheWrite != nil {
		cacheWrite = *sheet.Rates.CacheWrite
	}
	return &piModelCost{Input: sheet.Rates.Input, Output: sheet.Rates.Output, CacheRead: cacheRead, CacheWrite: cacheWrite}
}
