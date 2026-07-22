package main

import (
	"net/http"
	"strings"
)

type systemAdminRoute struct {
	method  string
	handler http.HandlerFunc
}

// SystemAdminAPI owns elevated operational controls and gateway-user
// administration, separate from provider credential operations.
type SystemAdminAPI struct {
	authorizeAdmin func(http.ResponseWriter, *http.Request) bool
	routes         map[string]systemAdminRoute
	poolUsers      http.HandlerFunc
}

func (api *SystemAdminAPI) TryServe(w http.ResponseWriter, r *http.Request) bool {
	if api == nil || r == nil {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/admin/pool-users") {
		if !api.authorizeAdmin(w, r) {
			return true
		}
		api.poolUsers(w, r)
		return true
	}
	route, ok := api.routes[r.URL.Path]
	if !ok {
		return false
	}
	if !api.authorizeAdmin(w, r) {
		return true
	}
	if route.method != "" && !requireMethod(w, r, route.method) {
		return true
	}
	route.handler(w, r)
	return true
}

func (h *proxyHandler) systemAdminAPIService() *SystemAdminAPI {
	if h.systemAdminAPI != nil {
		return h.systemAdminAPI
	}
	return &SystemAdminAPI{
		authorizeAdmin: h.checkAdminAuth,
		poolUsers:      h.servePoolUsersAdmin,
		routes: map[string]systemAdminRoute{
			"/metrics": {handler: h.metrics.serve},
			"/admin/reload": {method: http.MethodPost, handler: func(w http.ResponseWriter, _ *http.Request) {
				h.reloadAccounts()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}},
			"/admin/origins":           {method: http.MethodGet, handler: h.handleAdminOrigins},
			"/admin/tokens":            {method: http.MethodGet, handler: func(w http.ResponseWriter, _ *http.Request) { h.serveTokenCapacity(w) }},
			"/admin/clear-rate-limits": {method: http.MethodPost, handler: func(w http.ResponseWriter, _ *http.Request) { h.clearAllRateLimits(w) }},
			"/admin/purge-anonymous":   {method: http.MethodPost, handler: func(w http.ResponseWriter, _ *http.Request) { h.purgeAnonymousUsers(w) }},
		},
	}
}
