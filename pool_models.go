package main

import (
	"net/http"
	"strings"
	"time"
)

type poolModelDescriptor struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name,omitempty"`
	Protocol           string          `json:"protocol"`
	ContextWindow      int             `json:"contextWindow,omitempty"`
	Description        string          `json:"description,omitempty"`
	Provider           string          `json:"provider,omitempty"`
	UpstreamID         string          `json:"upstream_id,omitempty"`
	MaxOutputTokens    int             `json:"max_output_tokens,omitempty"`
	Protocols          []string        `json:"protocols,omitempty"`
	Modalities         []string        `json:"modalities,omitempty"`
	Capabilities       map[string]bool `json:"capabilities,omitempty"`
	SupportedMimeTypes []string        `json:"supported_mime_types,omitempty"`
	Recommended        bool            `json:"recommended,omitempty"`
	QuotaRemaining     *float64        `json:"quota_remaining_fraction,omitempty"`
	Aliases            []string        `json:"aliases,omitempty"`
	SupportingAccounts int             `json:"supporting_accounts,omitempty"`
	AvailableAccounts  int             `json:"available_accounts,omitempty"`
	AvailableNow       bool            `json:"available_now"`
	NextResetAt        *time.Time      `json:"next_reset_at,omitempty"`
	Stale              bool            `json:"stale,omitempty"`
}

func poolModelDescriptors(pools ...*ProviderPool) []poolModelDescriptor {
	var pool *ProviderPool
	if len(pools) > 0 {
		pool = pools[0]
	}
	models := make([]poolModelDescriptor, 0, len(poolModels)+len(grokModelCatalog))
	for _, model := range poolModels {
		supportingAccounts, availableAccounts, availableNow := poolModelAvailability(pool, model.ProviderID)
		protocol := "anthropic"
		if model.ProviderID == AccountTypeCodex {
			protocol = "openai"
		}
		models = append(models, poolModelDescriptor{
			ID:                 model.ID,
			Name:               model.DisplayName,
			Protocol:           protocol,
			ContextWindow:      model.ContextWindow,
			Description:        model.Description,
			Provider:           string(model.ProviderID),
			UpstreamID:         model.ID,
			MaxOutputTokens:    model.MaxTokens,
			Protocols:          []string{protocol},
			Modalities:         append([]string(nil), model.Input...),
			Capabilities:       map[string]bool{"reasoning": model.Reasoning, "tools": true},
			Aliases:            append([]string(nil), model.Aliases...),
			SupportingAccounts: supportingAccounts,
			AvailableAccounts:  availableAccounts,
			AvailableNow:       availableNow,
		})
	}
	for _, model := range grokModelCatalog {
		supportingAccounts, availableAccounts, availableNow := poolModelAvailability(pool, AccountTypeGrok)
		models = append(models, poolModelDescriptor{
			ID:                 model.ID,
			Name:               model.Name,
			Protocol:           "openai",
			ContextWindow:      model.ContextWindow,
			Provider:           string(AccountTypeGrok),
			UpstreamID:         model.ID,
			MaxOutputTokens:    model.MaxTokens,
			Protocols:          []string{"openai"},
			Capabilities:       map[string]bool{"reasoning": model.Reasoning, "tools": true},
			Aliases:            append([]string(nil), model.Aliases...),
			SupportingAccounts: supportingAccounts,
			AvailableAccounts:  availableAccounts,
			AvailableNow:       availableNow,
		})
	}
	for _, model := range antigravityModels.Models(pool) {
		modalities := []string{"text"}
		if model.SupportsImages {
			modalities = append(modalities, "image")
		}
		canonicalID := "antigravity/" + model.ID
		aliases := antigravityDescriptorAliases(canonicalID, model.ID, model.Aliases)
		models = append(models, poolModelDescriptor{
			ID: canonicalID, Name: model.DisplayName, Protocol: "openai",
			ContextWindow: model.MaxTokens, Provider: string(AccountTypeAntigravity), UpstreamID: model.ID,
			MaxOutputTokens: model.MaxOutputTokens, Protocols: []string{"gemini", "openai", "responses", "anthropic"},
			Modalities: modalities, Capabilities: map[string]bool{"reasoning": model.SupportsThinking, "images": model.SupportsImages, "tools": true, "web_search": model.WebSearch},
			SupportedMimeTypes: append([]string(nil), model.SupportedMimeTypes...), Recommended: model.Recommended, QuotaRemaining: model.Quota.RemainingFraction,
			Aliases: aliases, SupportingAccounts: model.SupportingAccounts, AvailableAccounts: model.AvailableAccounts,
			AvailableNow: model.AvailableNow, NextResetAt: optionalModelReset(model.NextResetAt), Stale: model.Stale,
		})
	}
	return models
}

func antigravityDescriptorAliases(canonicalID, upstreamID string, aliases []string) []string {
	all := append([]string(nil), aliases...)
	if !poolModelIDExists(upstreamID) {
		all = append(all, upstreamID)
	}
	seen := map[string]bool{strings.ToLower(canonicalID): true}
	out := make([]string, 0, len(all))
	for _, alias := range all {
		alias = strings.TrimSpace(alias)
		key := strings.ToLower(alias)
		if alias == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, alias)
	}
	return out
}

func optionalModelReset(reset time.Time) *time.Time {
	if reset.IsZero() {
		return nil
	}
	return &reset
}

func poolModelAvailability(pool *ProviderPool, accountType AccountType) (int, int, bool) {
	if pool == nil {
		return 0, 0, true
	}
	now := time.Now()
	supportingAccounts := 0
	availableAccounts := 0
	for _, account := range pool.allAccounts() {
		if account.Type != accountType {
			continue
		}
		supportingAccounts++
		account.mu.Lock()
		if accountAvailableForRoutingLocked(account, now) && !account.NeedsVerification {
			availableAccounts++
		}
		account.mu.Unlock()
	}
	return supportingAccounts, availableAccounts, availableAccounts > 0
}

func poolModelIDExists(id string) bool {
	for _, model := range poolModels {
		if strings.EqualFold(model.ID, id) {
			return true
		}
	}
	for _, model := range grokModelCatalog {
		if strings.EqualFold(model.ID, id) {
			return true
		}
	}
	return false
}

func poolModelDescriptorsWithRegistry(pool *ProviderPool, registry *ProviderRegistry) []poolModelDescriptor {
	models := poolModelDescriptors(pool)
	if registry == nil {
		return models
	}
	index := make(map[string]int, len(models))
	for modelIndex, model := range models {
		index[strings.ToLower(model.ID)] = modelIndex
	}
	for _, provider := range registry.DeclarativeProviders() {
		spec := provider.Spec()
		protocol := "anthropic"
		if spec.Protocol == ProtocolOpenAIChat {
			protocol = "openai"
		}
		for _, model := range spec.Models {
			supporting, available, availableNow := poolModelAvailability(pool, spec.ID)
			name := model.DisplayName
			if name == "" {
				name = model.ID
			}
			modalities := append([]string(nil), model.Input...)
			if len(modalities) == 0 {
				modalities = []string{"text"}
			}
			descriptor := poolModelDescriptor{
				ID: model.ID, Name: name, Description: model.Description,
				Protocol: protocol, Protocols: []string{protocol}, Provider: string(spec.ID), UpstreamID: model.ID,
				ContextWindow: model.ContextWindow, MaxOutputTokens: model.MaxOutputTokens,
				Modalities: modalities, Capabilities: map[string]bool{"reasoning": model.Reasoning, "tools": true},
				Aliases: append([]string(nil), model.Aliases...), SupportingAccounts: supporting, AvailableAccounts: available, AvailableNow: availableNow,
			}
			key := strings.ToLower(model.ID)
			if existing, ok := index[key]; ok {
				models[existing] = descriptor
			} else {
				index[key] = len(models)
				models = append(models, descriptor)
			}
		}
	}
	return models
}

func servePoolModels(w http.ResponseWriter, pools ...*ProviderPool) {
	var pool *ProviderPool
	if len(pools) > 0 {
		pool = pools[0]
	}
	servePoolModelsWithRegistry(w, pool, nil)
}

func servePoolModelsWithRegistry(w http.ResponseWriter, pool *ProviderPool, registry *ProviderRegistry) {
	respondJSON(w, map[string]any{"models": poolModelDescriptorsWithRegistry(pool, registry)})
}

func serveUnifiedOpenAIModels(w http.ResponseWriter, pools ...*ProviderPool) {
	var pool *ProviderPool
	if len(pools) > 0 {
		pool = pools[0]
	}
	serveUnifiedOpenAIModelsWithRegistry(w, pool, nil)
}

func serveUnifiedOpenAIModelsWithRegistry(w http.ResponseWriter, pool *ProviderPool, registry *ProviderRegistry) {
	descriptors := poolModelDescriptorsWithRegistry(pool, registry)
	data := make([]map[string]any, 0, len(descriptors))
	seen := make(map[string]bool)
	for _, model := range descriptors {
		if seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		data = append(data, map[string]any{"id": model.ID, "object": "model", "created": int64(0), "owned_by": model.Provider})
	}
	respondJSON(w, map[string]any{"object": "list", "data": data})
}

func serveUnifiedGeminiModels(w http.ResponseWriter, pool *ProviderPool) {
	models := make([]map[string]any, 0)
	for _, model := range antigravityModels.Models(pool) {
		methods := []string{"generateContent", "streamGenerateContent", "countTokens"}
		models = append(models, map[string]any{
			"name": "models/" + model.ID, "displayName": model.DisplayName,
			"inputTokenLimit": model.MaxTokens, "outputTokenLimit": model.MaxOutputTokens,
			"supportedGenerationMethods": methods,
		})
	}
	respondJSON(w, map[string]any{"models": models})
}
