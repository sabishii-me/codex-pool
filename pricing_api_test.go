package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelPricingEndpointReportsKnownAndUnknownWithoutInventingZero(t *testing.T) {
	h, user, secret := newTestHandlerWithSession(t)
	h.pricing = &PricingData{models: map[string]ModelPricing{
		"deepseek-v4-pro": {InputCostPerToken: 0.00000027, OutputCostPerToken: 0.0000011, CacheReadCost: 0.00000007, cacheReadSet: true},
	}, source: "test_fixture"}

	request := httptest.NewRequest(http.MethodGet, "/api/v2/pricing/models/deepseek-v4-pro", nil)
	request.AddCookie(newTestSessionCookie(t, secret, user))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("known pricing status=%d body=%s", response.Code, response.Body.String())
	}
	var known ModelPriceSheet
	if err := json.Unmarshal(response.Body.Bytes(), &known); err != nil {
		t.Fatal(err)
	}
	if known.Status != "known" || known.Rates == nil || known.Rates.Input != 0.27 || known.Rates.Output != 1.1 || known.Rates.CacheRead == nil || *known.Rates.CacheRead != 0.07 || known.Rates.CacheWrite != nil || known.Source.Kind != "test_fixture" {
		t.Fatalf("known pricing = %#v", known)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v2/pricing/models/gpt-5.6-sol", nil)
	request.AddCookie(newTestSessionCookie(t, secret, user))
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unknown pricing status=%d body=%s", response.Code, response.Body.String())
	}
	var unknown ModelPriceSheet
	if err := json.Unmarshal(response.Body.Bytes(), &unknown); err != nil {
		t.Fatal(err)
	}
	if unknown.Status != "unknown" || unknown.Rates != nil || unknown.Reason == "" {
		t.Fatalf("unknown pricing = %#v", unknown)
	}
}

func TestModelPricingEndpointRequiresSessionAndSupportsProviderFilter(t *testing.T) {
	h, user, secret := newTestHandlerWithSession(t)
	h.pricing = &PricingData{models: map[string]ModelPricing{}}

	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v2/pricing/models", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", response.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v2/pricing/models?provider=deepseek", nil)
	request.AddCookie(newTestSessionCookie(t, secret, user))
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("filtered status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Models []ModelPriceSheet `json:"models"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Models) != 2 {
		t.Fatalf("deepseek price sheets=%d, want 2", len(result.Models))
	}
	for _, sheet := range result.Models {
		if sheet.ProviderID != AccountTypeDeepSeek {
			t.Fatalf("unexpected provider %q", sheet.ProviderID)
		}
	}
}

func TestPiModelsUseCanonicalPricingAndOmitUnknownCost(t *testing.T) {
	pricing := &PricingData{models: map[string]ModelPricing{
		"deepseek-v4-pro": {InputCostPerToken: 0.00000027, OutputCostPerToken: 0.0000011, CacheReadCost: 0.00000007, cacheReadSet: true},
	}, source: "test_fixture"}
	models := piModelsForProvider(AccountTypeDeepSeek, pricing)
	if len(models) != 2 {
		t.Fatalf("deepseek model count=%d", len(models))
	}
	for _, model := range models {
		switch model.ID {
		case "deepseek-v4-pro":
			if model.Cost == nil || model.Cost.Input != 0.27 || model.Cost.Output != 1.1 {
				t.Fatalf("known Pi cost=%#v", model.Cost)
			}
		case "deepseek-v4-flash":
			if model.Cost != nil {
				t.Fatalf("unknown Pi cost must be omitted, got %#v", model.Cost)
			}
		}
	}
}
