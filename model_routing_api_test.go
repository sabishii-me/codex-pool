package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestModelRoutingProjectionReportsEligibilityAndExactExclusions(t *testing.T) {
	now := time.Now()
	eligible := &ProviderConnection{ID: "eligible", Type: AccountTypeCodex, Identity: ConnectionIdentity{DisplayName: "Ready"}, PlanType: "plus"}
	disabled := &ProviderConnection{ID: "disabled", Type: AccountTypeCodex, Identity: ConnectionIdentity{DisplayName: "Off"}, Disabled: true}
	cooldown := &ProviderConnection{ID: "cooldown", Type: AccountTypeCodex, Identity: ConnectionIdentity{DisplayName: "Cooling"}, RateLimitUntil: now.Add(time.Hour)}
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{})
	h := &proxyHandler{pool: newProviderPool([]*ProviderConnection{eligible, disabled, cooldown}, false), registry: registry, modelRoutes: NewModelRouteRegistry(registry)}
	response := httptest.NewRecorder()
	h.serveModelRouting(response, httptest.NewRequest(http.MethodGet, "/api/v2/models/gpt-5.6-sol/routing", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var projection ModelRoutingProjection
	if err := json.Unmarshal(response.Body.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	if projection.SelectionMode != "request_time" || projection.ProviderID != AccountTypeCodex || len(projection.EligibleConnections) != 1 || len(projection.ExcludedConnections) != 2 {
		t.Fatalf("projection=%#v", projection)
	}
	reasons := map[string]bool{}
	for _, connection := range projection.ExcludedConnections {
		reasons[connection.Reason] = true
	}
	if !reasons["disabled"] || !reasons["rate-limit cooldown"] {
		t.Fatalf("reasons=%v", reasons)
	}
}

func TestModelRoutingProjectionRejectsUnknownModel(t *testing.T) {
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{})
	h := &proxyHandler{pool: newProviderPool(nil, false), registry: registry, modelRoutes: NewModelRouteRegistry(registry)}
	response := httptest.NewRecorder()
	h.serveModelRouting(response, httptest.NewRequest(http.MethodGet, "/api/v2/models/not-real/routing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
