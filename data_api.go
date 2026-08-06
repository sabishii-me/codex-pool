package main

import (
	"net/http"
	"strings"
	"time"
)

// DataAPI owns routing for read-only gateway data endpoints. Authentication is
// injected so this layer does not inspect sessions, administrator state, or
// mutable proxy transport internals.
type DataAPI struct {
	authorizeSession func(http.ResponseWriter, *http.Request) bool
	authorizeAdmin   func(http.ResponseWriter, *http.Request) bool

	poolStats           http.HandlerFunc
	poolUsers           http.HandlerFunc
	signalAnalytics     http.HandlerFunc
	modelCatalog        http.HandlerFunc
	usageV2             http.HandlerFunc
	usageEconomicsV2    http.HandlerFunc
	pricingModels       http.HandlerFunc
	modelRouting        http.HandlerFunc
	providerConnections func(http.ResponseWriter)
	legacyConnections   func(http.ResponseWriter)

	// readCache deduplicates expensive read-only analytics endpoints so many
	// concurrent clients never issue identical heavy queries.
	readCache *ttlSingleFlight
}

// TryServe handles a request when its path belongs to the read-only data API.
// It returns false for mutation, authentication, static, and gateway paths.
func (api *DataAPI) TryServe(w http.ResponseWriter, r *http.Request) bool {
	if api == nil || r == nil {
		return false
	}

	if strings.HasPrefix(r.URL.Path, "/api/v2/models/") && strings.HasSuffix(r.URL.Path, "/routing") {
		if !api.authorizeAdmin(w, r) {
			return true
		}
		if !requireMethod(w, r, http.MethodGet) {
			return true
		}
		api.modelRouting(w, r)
		return true
	}

	if r.URL.Path == "/api/v2/pricing/models" || strings.HasPrefix(r.URL.Path, "/api/v2/pricing/models/") {
		api.pricingModels(w, r)
		return true
	}

	switch r.URL.Path {
	case "/api/pool/stats":
		return api.serveSession(w, r, api.poolStats)
	case "/api/pool/users":
		return api.serveSession(w, r, api.poolUsers)
	case "/api/pool/signal":
		return api.serveSession(w, r, api.signalAnalytics)
	case "/api/pool/catalog":
		if !api.authorizeSession(w, r) {
			return true
		}
		if !requireMethod(w, r, http.MethodGet) {
			return true
		}
		api.modelCatalog(w, r)
		return true
	case "/api/v2/usage/economics":
		api.usageEconomicsV2(w, r)
		return true
	case "/api/v2/usage":
		api.usageV2(w, r)
		return true
	case "/api/v2/provider-connections":
		if !api.authorizeAdmin(w, r) {
			return true
		}
		if !requireMethod(w, r, http.MethodGet) {
			return true
		}
		api.providerConnections(w)
		return true
	case "/admin/accounts":
		if !api.authorizeAdmin(w, r) {
			return true
		}
		if !requireMethod(w, r, http.MethodGet) {
			return true
		}
		api.legacyConnections(w)
		return true
	}

	// Preserve compatibility behavior: these handlers historically owned
	// method validation (if any), so this routing extraction does not add one.
	return false
}

func (api *DataAPI) serveSession(w http.ResponseWriter, r *http.Request, handler http.HandlerFunc) bool {
	if !api.authorizeSession(w, r) {
		return true
	}
	handler(w, r)
	return true
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func (h *proxyHandler) dataAPIService() *DataAPI {
	if h.dataAPI != nil {
		return h.dataAPI
	}
	api := &DataAPI{
		authorizeSession: h.checkAdminOrSessionAuth,
		authorizeAdmin:   h.checkAdminAuth,
		poolUsers:        h.handlePoolUsers,
		modelCatalog: func(w http.ResponseWriter, _ *http.Request) {
			servePoolModelsWithRegistry(w, h.pool, h.registry)
		},
		pricingModels:       h.handleModelPricingV2,
		modelRouting:        h.serveModelRouting,
		providerConnections: h.serveProviderConnectionsV2,
		legacyConnections:   h.serveAccounts,
		readCache:           newTTLSingleFlight(),
	}
	// Expensive read-only endpoints are single-flighted and short-TTL cached so
	// many concurrent clients share one backend computation instead of issuing
	// identical heavy queries. Caching runs after authorization.
	api.poolStats = api.cachedHandler(5*time.Second, cacheAllGET, h.handlePoolStats)
	api.signalAnalytics = api.cachedHandler(5*time.Second, cacheAllGET, h.handleSignalAnalytics)
	api.usageEconomicsV2 = api.cachedHandler(10*time.Second, cacheAllGET, h.handleUsageEconomicsV2)
	api.usageV2 = api.cachedHandler(5*time.Second, cachePoolUsageOnly, h.handleUsageV2)
	h.dataAPI = api
	return api
}
