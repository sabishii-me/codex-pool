package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

type systemAdminRoute struct {
	method  string
	handler http.HandlerFunc
}

type systemProjection struct {
	Evidence struct {
		Kind        string    `json:"kind"`
		Source      string    `json:"source"`
		GeneratedAt time.Time `json:"generated_at"`
	} `json:"evidence"`
	Runtime struct {
		Status        string    `json:"status"`
		Version       string    `json:"version"`
		Commit        string    `json:"commit"`
		BuildDate     string    `json:"build_date"`
		StartedAt     time.Time `json:"started_at"`
		UptimeSeconds int64     `json:"uptime_seconds"`
	} `json:"runtime"`
	Capacity struct {
		ConnectionsTotal  int `json:"connections_total"`
		ConnectionsActive int `json:"connections_active"`
		ConnectionsDead   int `json:"connections_dead"`
		ConnectionsOff    int `json:"connections_disabled"`
		Providers         int `json:"providers_registered"`
		Declarative       int `json:"declarative_providers"`
	} `json:"capacity"`
	Persistence []systemPersistenceProjection `json:"persistence"`
}

type systemPersistenceProjection struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Healthy    bool   `json:"healthy"`
	Detail     string `json:"detail"`
}

func (h *proxyHandler) serveSystemProjection(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().UTC()
	projection := systemProjection{}
	projection.Evidence.Kind, projection.Evidence.Source, projection.Evidence.GeneratedAt = "measured", "gateway runtime", now
	projection.Runtime.Status, projection.Runtime.Version, projection.Runtime.Commit, projection.Runtime.BuildDate, projection.Runtime.StartedAt = "ok", effectiveBuildVersion(), buildCommit, buildDate, h.startTime.UTC()
	projection.Runtime.UptimeSeconds = int64(time.Since(h.startTime).Seconds())
	for _, connection := range h.pool.allAccounts() {
		projection.Capacity.ConnectionsTotal++
		connection.mu.Lock()
		switch {
		case connection.Dead:
			projection.Capacity.ConnectionsDead++
		case connection.Disabled:
			projection.Capacity.ConnectionsOff++
		default:
			projection.Capacity.ConnectionsActive++
		}
		connection.mu.Unlock()
	}
	if h.registry != nil {
		projection.Capacity.Providers = len(h.registry.All())
		projection.Capacity.Declarative = len(h.registry.DeclarativeProviders())
	}
	usage := systemPersistenceProjection{Name: "Usage ledger", Configured: h.store != nil, Detail: "Exactly-once usage store"}
	if h.store != nil && h.store.db != nil {
		usage.Healthy = h.store.db.View(func(_ *bbolt.Tx) error { return nil }) == nil
	}
	analytics := systemPersistenceProjection{Name: "Analytics projection", Configured: h.analyticsStore != nil, Detail: "SQLite analytics store"}
	if h.analyticsStore != nil && h.analyticsStore.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		analytics.Healthy = h.analyticsStore.db.PingContext(ctx) == nil
		cancel()
	}
	members := systemPersistenceProjection{Name: "Gateway users", Configured: h.poolUsers != nil, Healthy: h.poolUsers != nil, Detail: "Authorized member store"}
	projection.Persistence = []systemPersistenceProjection{usage, analytics, members}
	respondJSON(w, projection)
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
			"/api/v2/system": {method: http.MethodGet, handler: h.serveSystemProjection},
			"/api/v2/system/reload-connections": {method: http.MethodPost, handler: func(w http.ResponseWriter, _ *http.Request) {
				h.reloadAccounts()
				respondJSON(w, map[string]string{"status": "ok"})
			}},
			"/api/v2/system/clear-rate-limits": {method: http.MethodPost, handler: func(w http.ResponseWriter, _ *http.Request) { h.clearAllRateLimits(w) }},
			"/metrics":                         {handler: h.metrics.serve},
			"/admin/reload": {method: http.MethodPost, handler: func(w http.ResponseWriter, _ *http.Request) {
				h.reloadAccounts()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}},
			"/admin/tokens":            {method: http.MethodGet, handler: func(w http.ResponseWriter, _ *http.Request) { h.serveTokenCapacity(w) }},
			"/admin/clear-rate-limits": {method: http.MethodPost, handler: func(w http.ResponseWriter, _ *http.Request) { h.clearAllRateLimits(w) }},
			"/admin/purge-anonymous":   {method: http.MethodPost, handler: func(w http.ResponseWriter, _ *http.Request) { h.purgeAnonymousUsers(w) }},
		},
	}
}
