package main

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const pricingUnitPerMillionTokens = "per_million_tokens"

type ModelPriceRates struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

type ModelPriceSource struct {
	Kind             string    `json:"kind"`
	ReferenceModelID string    `json:"reference_model_id,omitempty"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
}

type ModelPriceSheet struct {
	ModelID    string           `json:"model_id"`
	ProviderID ProviderID       `json:"provider_id"`
	Currency   string           `json:"currency"`
	Unit       string           `json:"unit"`
	Status     string           `json:"status"`
	RateKind   string           `json:"rate_kind,omitempty"`
	Rates      *ModelPriceRates `json:"rates,omitempty"`
	Source     ModelPriceSource `json:"source"`
	Reason     string           `json:"reason,omitempty"`
}

func priceSheetForModel(pricing *PricingData, model ModelRoute) ModelPriceSheet {
	sheet := ModelPriceSheet{
		ModelID: model.ID, ProviderID: model.ProviderID, Currency: "USD",
		Unit: pricingUnitPerMillionTokens, Status: "unknown",
		Source: ModelPriceSource{Kind: "unavailable"},
		Reason: "no verified pricing mapping",
	}
	if model.Cost != nil {
		sheet.Status = "known"
		sheet.RateKind = pricingRateKind(model.ProviderID)
		sheet.Rates = &ModelPriceRates{Input: model.Cost.Input, Output: model.Cost.Output, CacheRead: model.Cost.CacheRead, CacheWrite: model.Cost.CacheWrite}
		sheet.Source = ModelPriceSource{Kind: "gateway_catalog", ReferenceModelID: model.ID}
		sheet.Reason = ""
		return sheet
	}
	if pricing == nil {
		return sheet
	}
	price, source, updatedAt, ok := pricing.lookupExactPricing(model.ID)
	if !ok {
		return sheet
	}
	sheet.Status = "known"
	sheet.RateKind = pricingRateKind(model.ProviderID)
	sheet.Rates = &ModelPriceRates{
		Input:      price.InputCostPerToken * 1_000_000,
		Output:     price.OutputCostPerToken * 1_000_000,
		CacheRead:  price.CacheReadCost * 1_000_000,
		CacheWrite: price.CacheWriteCost * 1_000_000,
	}
	sheet.Source = ModelPriceSource{Kind: source, ReferenceModelID: model.ID, UpdatedAt: updatedAt}
	sheet.Reason = ""
	return sheet
}

func pricingRateKind(provider ProviderID) string {
	if provider == AccountTypeCodex || provider == AccountTypeKimi || provider == AccountTypeQwen {
		return "api_reference"
	}
	return "provider_billed"
}

func priceSheets(pricing *PricingData, provider string) []ModelPriceSheet {
	provider = strings.ToLower(strings.TrimSpace(provider))
	out := make([]ModelPriceSheet, 0, len(poolModels))
	for _, model := range poolModels {
		if provider != "" && strings.ToLower(string(model.ProviderID)) != provider {
			continue
		}
		out = append(out, priceSheetForModel(pricing, model))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProviderID != out[j].ProviderID {
			return out[i].ProviderID < out[j].ProviderID
		}
		return out[i].ModelID < out[j].ModelID
	})
	return out
}

func findModelRouteByID(id string) (ModelRoute, bool) {
	for _, model := range poolModels {
		if model.ID == id {
			return model, true
		}
	}
	return ModelRoute{}, false
}

func (h *proxyHandler) handleModelPricingV2(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if _, ok := h.sessionUser(r); !ok {
		respondJSONError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	const prefix = "/api/v2/pricing/models"
	if r.URL.Path == prefix {
		respondJSON(w, map[string]any{
			"currency": "USD", "unit": pricingUnitPerMillionTokens,
			"generated_at": time.Now().UTC(),
			"models":       priceSheets(h.pricing, r.URL.Query().Get("provider")),
		})
		return
	}
	if !strings.HasPrefix(r.URL.Path, prefix+"/") {
		respondJSONError(w, http.StatusNotFound, "pricing model not found")
		return
	}
	id, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, prefix+"/"))
	if err != nil || strings.TrimSpace(id) == "" || strings.Contains(id, "/") {
		respondJSONError(w, http.StatusBadRequest, "invalid model ID")
		return
	}
	model, ok := findModelRouteByID(id)
	if !ok {
		respondJSONError(w, http.StatusNotFound, "pricing model not found")
		return
	}
	respondJSON(w, priceSheetForModel(h.pricing, model))
}
