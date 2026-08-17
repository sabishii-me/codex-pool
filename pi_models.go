package main

import (
	"encoding/json"
	"os"
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

func generatePiModelsJSON(publicURL, codexAPIKey, anthropicAPIKey string, pool *ProviderPool, registry *ProviderRegistry, pricings ...*PricingData) ([]byte, error) {
	var pricing *PricingData
	if len(pricings) > 0 {
		pricing = pricings[0]
	}
	baseURL := strings.TrimRight(strings.TrimSpace(publicURL), "/")
	// The provider name is the pool's own identity (POOL_NAME). It must never
	// collide with a built-in Pi/OpenAI provider name (codex, antigravity, ...)
	// or the generated config would silently overwrite the official provider.
	providerName := strings.TrimSpace(os.Getenv("POOL_NAME"))
	if providerName == "" {
		providerName = "codex-pool"
	}
	cfg := piModelsConfig{
		Providers: map[string]piProviderConfig{
			providerName: {
				BaseURL: baseURL,
				APIKey:  anthropicAPIKey,
				API:     "anthropic-messages",
				Models:  availablePiModels(pool, registry, pricing),
			},
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

// availablePiModels returns only the pool models whose provider currently has
// at least one healthy, routable account. Providers without any available
// account are omitted so Pi never receives a model list with entries it cannot
// actually use (e.g. "displayed unavailable" models).
//
// Model data comes from the declarative provider specs (data-driven, provider-specs
// dir) rather than the hardcoded catalog; plugin providers that still keep their
// model catalog in code (codex/grok/kimi) are appended from poolModels until they
// are migrated to declarative specs too.
func availablePiModels(pool *ProviderPool, registry *ProviderRegistry, pricing *PricingData) []piModelConfig {
	var result []piModelConfig
	seen := map[string]bool{}
	// Declarative providers (data-driven specs) — the single source for standard
	// providers. Their models carry the authoritative context window / max output
	// / reasoning / input metadata from provider-specs.
	if registry != nil {
		for _, provider := range registry.DeclarativeProviders() {
			spec := provider.Spec()
			_, _, available := poolModelAvailability(pool, spec.ID)
			if !available {
				continue
			}
			for _, m := range spec.Models {
				if seen[m.ID] {
					continue
				}
				seen[m.ID] = true
				config := piModelConfig{
					ID:            m.ID,
					Name:          m.DisplayName,
					Reasoning:     boolPtr(m.Reasoning),
					Input:         append([]string(nil), m.Input...),
					ContextWindow: m.ContextWindow,
					MaxTokens:     m.MaxOutputTokens,
				}
				if pricing != nil {
					config.Cost = piCostForModel(pricing, modelRouteFromSpec(spec, m))
				}
				if spec.ID == AccountTypeCodex && strings.HasPrefix(m.ID, "gpt-5.6-") {
					config.ThinkingLevelMap = map[string]string{"xhigh": "xhigh", "max": "max"}
				}
				result = append(result, config)
			}
		}
	}
	// Plugin providers still backed by the hardcoded catalog until migrated.
	for _, model := range poolModels {
		if model.ProviderID != AccountTypeCodex && model.ProviderID != AccountTypeGrok && model.ProviderID != AccountTypeKimi {
			continue
		}
		if seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		_, _, available := poolModelAvailability(pool, model.ProviderID)
		if !available {
			continue
		}
		config := piModelConfig{
			ID:            model.ID,
			Name:          model.DisplayName,
			Reasoning:     boolPtr(model.Reasoning),
			Input:         append([]string(nil), model.Input...),
			ContextWindow: model.ContextWindow,
			MaxTokens:     model.MaxTokens,
			Cost:          piCostForModel(pricing, model),
		}
		if model.ProviderID == AccountTypeCodex && strings.HasPrefix(model.ID, "gpt-5.6-") {
			config.ThinkingLevelMap = map[string]string{"xhigh": "xhigh", "max": "max"}
		}
		result = append(result, config)
	}
	// Antigravity keeps its own model catalog with provider-prefixed IDs.
	_, _, available := poolModelAvailability(pool, AccountTypeAntigravity)
	if available {
		result = append(result, antigravityPiModels()...)
	}
	return result
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
