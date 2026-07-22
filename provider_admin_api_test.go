package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderAdminAPICanonicalAndLegacyIdentityShareMutation(t *testing.T) {
	var ids []string
	api := &ProviderAdminAPI{
		authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
		rename:         func(_ http.ResponseWriter, _ *http.Request, id string) { ids = append(ids, id) },
	}
	for _, path := range []string{
		"/api/v2/provider-connections/connection-1/identity",
		"/admin/accounts/connection-1/identity",
	} {
		if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodPatch, path, nil)) {
			t.Fatalf("route not claimed: %s", path)
		}
	}
	if len(ids) != 2 || ids[0] != "connection-1" || ids[1] != "connection-1" {
		t.Fatalf("ids=%v", ids)
	}
}

func TestProviderAdminAPIRoutesLifecycleActions(t *testing.T) {
	var action, id string
	var disabled bool
	api := &ProviderAdminAPI{
		authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
		setDisabled: func(_ http.ResponseWriter, gotID string, gotDisabled bool) {
			action, id, disabled = "disabled", gotID, gotDisabled
		},
		resurrect: func(_ http.ResponseWriter, gotID string) { action, id = "resurrect", gotID },
		refresh:   func(_ http.ResponseWriter, gotID string) { action, id = "refresh", gotID },
	}
	cases := []struct {
		path, wantAction string
		wantDisabled     bool
	}{
		{"/admin/accounts/c1/enable", "disabled", false},
		{"/admin/accounts/c1/disable", "disabled", true},
		{"/admin/accounts/c1/resurrect", "resurrect", false},
		{"/admin/accounts/c1/refresh", "refresh", false},
	}
	for _, test := range cases {
		action, id, disabled = "", "", false
		if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, test.path, nil)) {
			t.Fatalf("route not claimed: %s", test.path)
		}
		if action != test.wantAction || id != "c1" || disabled != test.wantDisabled {
			t.Fatalf("path=%s action=%s id=%s disabled=%v", test.path, action, id, disabled)
		}
	}
}

func TestProviderAdminAPIAuthorizesBeforeMethodAndMutation(t *testing.T) {
	mutated := false
	api := &ProviderAdminAPI{
		authorizeAdmin: func(w http.ResponseWriter, _ *http.Request) bool {
			http.Error(w, "sign in required", http.StatusUnauthorized)
			return false
		},
		refresh: func(http.ResponseWriter, string) { mutated = true },
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/admin/accounts/c1/refresh", nil)) {
		t.Fatal("route not claimed")
	}
	if response.Code != http.StatusUnauthorized || mutated {
		t.Fatalf("status=%d mutated=%v", response.Code, mutated)
	}
}

func TestProviderAdminAPIRejectsWrongMethodAndUnknownPaths(t *testing.T) {
	api := &ProviderAdminAPI{
		authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
		rename: func(http.ResponseWriter, *http.Request, string) {
			t.Fatal("invalid ID reached rename mutation")
		},
		setDisabled: func(http.ResponseWriter, string, bool) {
			t.Fatal("invalid ID reached disable mutation")
		},
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/admin/accounts/c1/disable", nil)) || response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status=%d", response.Code)
	}
	for _, path := range []string{"/admin/accounts", "/admin/accounts/c1/delete", "/api/v2/provider-connections", "/v1/messages"} {
		if api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil)) {
			t.Fatalf("claimed unknown/read/gateway path %s", path)
		}
	}
	for _, path := range []string{
		"/api/v2/provider-connections//identity",
		"/api/v2/provider-connections/nested/id/identity",
		"/admin/accounts//identity",
		"/admin/accounts//disable",
	} {
		method := http.MethodPatch
		if strings.HasSuffix(path, "/disable") {
			method = http.MethodPost
		}
		response := httptest.NewRecorder()
		if !api.TryServe(response, httptest.NewRequest(method, path, nil)) || response.Code != http.StatusBadRequest {
			t.Fatalf("invalid path=%s status=%d", path, response.Code)
		}
	}
}

func TestProxyHandlerDelegatesLifecycleMutationToProviderAdminAPI(t *testing.T) {
	called := false
	handler := &proxyHandler{
		cfg:     &config{},
		dataAPI: &DataAPI{},
		providerAdminAPI: &ProviderAdminAPI{
			authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
			refresh: func(w http.ResponseWriter, id string) {
				called = id == "connection-1"
				w.WriteHeader(http.StatusNoContent)
			},
		},
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/admin/accounts/connection-1/refresh", nil))
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, response.Code)
	}
}
