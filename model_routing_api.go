package main

import (
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type ModelRoutingConnection struct {
	PublicID    string `json:"public_id"`
	DisplayName string `json:"display_name"`
	PlanType    string `json:"plan_type,omitempty"`
	Inflight    int64  `json:"inflight"`
	Primary     bool   `json:"primary"`
	Reason      string `json:"reason,omitempty"`
}

type ModelRoutingProjection struct {
	RequestedModel      string                   `json:"requested_model"`
	CanonicalModel      string                   `json:"canonical_model"`
	ProviderID          ProviderID               `json:"provider_id"`
	SelectionMode       string                   `json:"selection_mode"`
	EligibleConnections []ModelRoutingConnection `json:"eligible_connections"`
	ExcludedConnections []ModelRoutingConnection `json:"excluded_connections"`
	Evidence            struct {
		Kind        string    `json:"kind"`
		Source      string    `json:"source"`
		GeneratedAt time.Time `json:"generated_at"`
	} `json:"evidence"`
}

func routingExclusionReasonLocked(connection *ProviderConnection, providerID ProviderID, model string, now time.Time) string {
	switch {
	case connection.Type != providerID:
		return "provider mismatch"
	case connection.Dead:
		return "dead"
	case connection.Disabled:
		return "disabled"
	case accountCoolingDownLocked(connection, now):
		return "rate-limit cooldown"
	case accountPrimaryUsageLocked(connection) >= primaryHardExcludeThreshold:
		return "primary quota exhausted"
	case accountSecondaryUsageLocked(connection) >= secondaryHardExcludeThreshold:
		return "secondary quota exhausted"
	case providerID == AccountTypeAntigravity && !antigravityModels.Supports(connection.ID, strings.TrimPrefix(model, "antigravity/")):
		return "model not available on connection"
	default:
		return ""
	}
}

func (h *proxyHandler) serveModelRouting(w http.ResponseWriter, r *http.Request) {
	modelID, err := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v2/models/"), "/routing"))
	if err != nil || strings.TrimSpace(modelID) == "" || strings.Contains(modelID, "/routing") {
		respondJSONError(w, http.StatusBadRequest, "invalid model ID")
		return
	}
	known := false
	for _, model := range poolModelDescriptorsWithRegistry(h.pool, h.registry) {
		if strings.EqualFold(model.ID, modelID) {
			known = true
			break
		}
		for _, alias := range model.Aliases {
			if strings.EqualFold(alias, modelID) {
				known = true
				break
			}
		}
		if known {
			break
		}
	}
	nativeModel, native := resolveNativeModel(modelID, WorkloadImageGeneration)
	route, routeOK := h.routeRegistry().Resolve("/v1/responses", modelID)
	var canonicalModel string
	var providerID ProviderID
	switch {
	case native:
		canonicalModel, providerID = nativeModel.ID, nativeModel.ProviderID
	case routeOK && route.Provider != nil:
		canonicalModel, providerID = route.CanonicalModel, route.Provider.Type()
	default:
		respondJSONError(w, http.StatusNotFound, "model route not found")
		return
	}
	if !known {
		respondJSONError(w, http.StatusNotFound, "model route not found")
		return
	}
	projection := ModelRoutingProjection{RequestedModel: modelID, CanonicalModel: canonicalModel, ProviderID: providerID, SelectionMode: "request_time", EligibleConnections: []ModelRoutingConnection{}, ExcludedConnections: []ModelRoutingConnection{}}
	projection.Evidence.Kind, projection.Evidence.Source, projection.Evidence.GeneratedAt = "runtime", "connection_selector_policy", time.Now().UTC()
	views := h.connectionViewService().OperatorConnections()
	primary := map[string]bool{}
	for _, view := range views {
		primary[view.ID] = view.IsPrimary
	}
	for _, connection := range h.pool.allAccounts() {
		connection.mu.Lock()
		if connection.Type != projection.ProviderID {
			connection.mu.Unlock()
			continue
		}
		identity := connection.connectionIdentityLocked()
		item := ModelRoutingConnection{PublicID: hashAccountID(connection.ID), DisplayName: identity.DisplayName, PlanType: connection.PlanType, Inflight: atomic.LoadInt64(&connection.Inflight), Primary: primary[connection.ID]}
		item.Reason = routingExclusionReasonLocked(connection, projection.ProviderID, canonicalModel, projection.Evidence.GeneratedAt)
		connection.mu.Unlock()
		if item.Reason == "" {
			projection.EligibleConnections = append(projection.EligibleConnections, item)
		} else {
			projection.ExcludedConnections = append(projection.ExcludedConnections, item)
		}
	}
	respondJSON(w, projection)
}
