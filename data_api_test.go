package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDataAPIRoutesReadCollectionsWithoutOwningMutations(t *testing.T) {
	called := ""
	api := &DataAPI{
		authorizeSession:    func(http.ResponseWriter, *http.Request) bool { return true },
		authorizeAdmin:      func(http.ResponseWriter, *http.Request) bool { return true },
		providerConnections: func(http.ResponseWriter) { called = "connections" },
		legacyConnections:   func(http.ResponseWriter) { called = "legacy" },
	}

	if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v2/provider-connections", nil)) || called != "connections" {
		t.Fatalf("canonical collection not routed, called=%q", called)
	}
	called = ""
	if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/admin/accounts", nil)) || called != "legacy" {
		t.Fatalf("legacy collection not routed, called=%q", called)
	}
	for _, path := range []string{
		"/api/v2/provider-connections/connection-1/identity",
		"/admin/accounts/connection-1/disable",
		"/api/pool/accounts/codex/add",
		"/v1/messages",
	} {
		if api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil)) {
			t.Fatalf("DataAPI claimed mutation/gateway path %s", path)
		}
	}
}

func TestDataAPIAuthorizesBeforeCallingReadHandler(t *testing.T) {
	handlerCalled := false
	api := &DataAPI{
		authorizeSession: func(w http.ResponseWriter, _ *http.Request) bool {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		},
		poolStats: func(http.ResponseWriter, *http.Request) { handlerCalled = true },
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/api/pool/stats", nil)) {
		t.Fatal("known route was not claimed")
	}
	if response.Code != http.StatusUnauthorized || handlerCalled {
		t.Fatalf("status=%d handlerCalled=%v", response.Code, handlerCalled)
	}
}

func TestDataAPIPreservesExplicitCollectionMethods(t *testing.T) {
	adminCalls := 0
	handlerCalled := false
	api := &DataAPI{
		authorizeAdmin: func(http.ResponseWriter, *http.Request) bool {
			adminCalls++
			return true
		},
		providerConnections: func(http.ResponseWriter) { handlerCalled = true },
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodPost, "/api/v2/provider-connections", nil)) {
		t.Fatal("known route was not claimed")
	}
	if response.Code != http.StatusMethodNotAllowed || adminCalls != 1 || handlerCalled {
		t.Fatalf("status=%d adminCalls=%d handlerCalled=%v", response.Code, adminCalls, handlerCalled)
	}
}

func TestDataAPIRoutesDynamicUserUsageReads(t *testing.T) {
	called := ""
	api := &DataAPI{
		authorizeSession: func(http.ResponseWriter, *http.Request) bool { return true },
		userDaily:        func(http.ResponseWriter, *http.Request) { called = "daily" },
		userHourly:       func(http.ResponseWriter, *http.Request) { called = "hourly" },
	}
	if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/pool/users/member-1/daily", nil)) || called != "daily" {
		t.Fatalf("daily route called=%q", called)
	}
	called = ""
	if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/pool/users/member-1/hourly", nil)) || called != "hourly" {
		t.Fatalf("hourly route called=%q", called)
	}
}

func TestProxyHandlerDelegatesProviderConnectionReadToDataAPI(t *testing.T) {
	called := false
	handler := &proxyHandler{
		cfg: &config{},
		dataAPI: &DataAPI{
			authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
			providerConnections: func(w http.ResponseWriter) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			},
		},
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v2/provider-connections", nil))
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, response.Code)
	}
}
