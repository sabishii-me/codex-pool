package main

import (
	"net/http"
	"strings"
)

// DataAPI owns routing for read-only gateway data endpoints. Authentication is
// injected so this layer does not inspect sessions, administrator state, or
// mutable proxy transport internals.
type DataAPI struct {
	authorizeSession func(http.ResponseWriter, *http.Request) bool
	authorizeAdmin   func(http.ResponseWriter, *http.Request) bool

	poolStats           http.HandlerFunc
	whoami              http.HandlerFunc
	poolUsers           http.HandlerFunc
	poolOrigins         http.HandlerFunc
	dailyBreakdown      http.HandlerFunc
	globalHourly        http.HandlerFunc
	signalAnalytics     http.HandlerFunc
	userDaily           http.HandlerFunc
	userHourly          http.HandlerFunc
	modelCatalog        http.HandlerFunc
	providerConnections func(http.ResponseWriter)
	legacyConnections   func(http.ResponseWriter)
}

// TryServe handles a request when its path belongs to the read-only data API.
// It returns false for mutation, authentication, static, and gateway paths.
func (api *DataAPI) TryServe(w http.ResponseWriter, r *http.Request) bool {
	if api == nil || r == nil {
		return false
	}

	switch r.URL.Path {
	case "/api/pool/stats":
		return api.serveSession(w, r, api.poolStats)
	case "/api/pool/whoami":
		api.whoami(w, r)
		return true
	case "/api/pool/users":
		return api.serveSession(w, r, api.poolUsers)
	case "/api/pool/origins":
		return api.serveSession(w, r, api.poolOrigins)
	case "/api/pool/daily-breakdown":
		return api.serveSession(w, r, api.dailyBreakdown)
	case "/api/pool/hourly":
		return api.serveSession(w, r, api.globalHourly)
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
	if strings.HasPrefix(r.URL.Path, "/api/pool/users/") {
		switch {
		case strings.HasSuffix(r.URL.Path, "/daily"):
			return api.serveSession(w, r, api.userDaily)
		case strings.HasSuffix(r.URL.Path, "/hourly"):
			return api.serveSession(w, r, api.userHourly)
		}
	}
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
	return &DataAPI{
		authorizeSession: h.checkAdminOrSessionAuth,
		authorizeAdmin:   h.checkAdminAuth,
		poolStats:        h.handlePoolStats,
		whoami:           h.handleWhoami,
		poolUsers:        h.handlePoolUsers,
		poolOrigins:      h.handlePoolOrigins,
		dailyBreakdown:   h.handleDailyBreakdown,
		globalHourly:     h.handleGlobalHourly,
		signalAnalytics:  h.handleSignalAnalytics,
		userDaily:        h.handleUserDaily,
		userHourly:       h.handleUserHourly,
		modelCatalog: func(w http.ResponseWriter, _ *http.Request) {
			servePoolModelsWithRegistry(w, h.pool, h.registry)
		},
		providerConnections: h.serveProviderConnectionsV2,
		legacyConnections:   h.serveAccounts,
	}
}
