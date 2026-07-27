package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSystemAdminAPIProjectionReportsMeasuredRuntime(t *testing.T) {
	handler := &proxyHandler{startTime: time.Now().Add(-time.Minute), pool: newProviderPool(nil), registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{})}
	response := httptest.NewRecorder()
	handler.serveSystemProjection(response, httptest.NewRequest(http.MethodGet, "/api/v2/system", nil))
	var projection systemProjection
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &projection) != nil {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if projection.Evidence.Kind != "measured" || projection.Runtime.UptimeSeconds < 59 || projection.Capacity.Providers != 3 {
		t.Fatalf("projection=%#v", projection)
	}
}

func TestSystemAdminAPIAuthorizesBeforeMethodAndHandler(t *testing.T) {
	called := false
	api := &SystemAdminAPI{
		authorizeAdmin: func(w http.ResponseWriter, _ *http.Request) bool {
			http.Error(w, "forbidden", http.StatusForbidden)
			return false
		},
		routes: map[string]systemAdminRoute{
			"/admin/reload": {method: http.MethodPost, handler: func(http.ResponseWriter, *http.Request) { called = true }},
		},
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/admin/reload", nil)) || response.Code != http.StatusForbidden || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
}

func TestSystemAdminAPIEnforcesDeclaredMethods(t *testing.T) {
	called := false
	api := &SystemAdminAPI{
		authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
		routes: map[string]systemAdminRoute{
			"/admin/reload": {method: http.MethodPost, handler: func(http.ResponseWriter, *http.Request) { called = true }},
		},
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/admin/reload", nil)) || response.Code != http.StatusMethodNotAllowed || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
	response = httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodPost, "/admin/reload", nil)) || !called {
		t.Fatalf("POST called=%v status=%d", called, response.Code)
	}
}

func TestSystemAdminAPIRoutesPoolUsersAfterAuthorization(t *testing.T) {
	called := false
	api := &SystemAdminAPI{
		authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
		poolUsers:      func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) },
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodDelete, "/admin/pool-users/member", nil)) || !called || response.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, response.Code)
	}
}

func TestSystemAdminAPIDoesNotClaimOtherBoundaries(t *testing.T) {
	api := &SystemAdminAPI{}
	for _, path := range []string{"/admin/codex", "/admin/accounts/c1/disable", "/api/pool/stats", "/v1/messages", "/admin/unknown"} {
		if api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil)) {
			t.Fatalf("claimed unrelated path %s", path)
		}
	}
}

func TestProxyHandlerDelegatesSystemAdminRoute(t *testing.T) {
	called := false
	handler := &proxyHandler{
		cfg:                     &config{},
		dataAPI:                 &DataAPI{},
		providerAdminAPI:        &ProviderAdminAPI{},
		providerContributionAPI: &ProviderContributionAPI{},
		providerOperationsAPI:   &ProviderOperationsAPI{},
		authenticationAPI:       &AuthenticationAPI{},
		systemAdminAPI: &SystemAdminAPI{
			authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
			routes: map[string]systemAdminRoute{
				"/admin/reload": {method: http.MethodPost, handler: func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) }},
			},
		},
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/admin/reload", nil))
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, response.Code)
	}
}
