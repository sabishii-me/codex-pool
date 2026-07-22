package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderContributionAPIAuthorizesBeforeMethodAndDispatch(t *testing.T) {
	dispatched := false
	api := &ProviderContributionAPI{
		authorizeSession: func(w http.ResponseWriter, _ *http.Request) bool {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		},
		handlers: map[string]http.HandlerFunc{
			"/api/pool/accounts/deepseek/add": func(http.ResponseWriter, *http.Request) { dispatched = true },
		},
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/api/pool/accounts/deepseek/add", nil)) {
		t.Fatal("known contribution namespace was not claimed")
	}
	if response.Code != http.StatusUnauthorized || dispatched {
		t.Fatalf("status=%d dispatched=%v", response.Code, dispatched)
	}
}

func TestProviderContributionAPIRequiresPostAndExactRoute(t *testing.T) {
	api := &ProviderContributionAPI{
		authorizeSession: func(http.ResponseWriter, *http.Request) bool { return true },
		handlers: map[string]http.HandlerFunc{
			"/api/pool/accounts/deepseek/add": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) },
		},
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/api/pool/accounts/deepseek/add", nil)) || response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodPost, "/api/pool/accounts/unknown/add", nil)) || response.Code != http.StatusNotFound {
		t.Fatalf("unknown status=%d", response.Code)
	}
	for _, path := range []string{"/api/pool/accounts", "/admin/accounts/c1/disable", "/v1/messages"} {
		if api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, nil)) {
			t.Fatalf("claimed unrelated path %s", path)
		}
	}
}

func TestProviderContributionAPIBoundsRequestBody(t *testing.T) {
	var readErr error
	api := &ProviderContributionAPI{
		authorizeSession: func(http.ResponseWriter, *http.Request) bool { return true },
		handlers: map[string]http.HandlerFunc{
			"/api/pool/accounts/deepseek/add": func(w http.ResponseWriter, r *http.Request) {
				_, readErr = io.ReadAll(r.Body)
				if readErr != nil {
					http.Error(w, readErr.Error(), http.StatusRequestEntityTooLarge)
				}
			},
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/pool/accounts/deepseek/add", strings.NewReader(strings.Repeat("x", providerContributionBodyLimit+1)))
	response := httptest.NewRecorder()
	api.TryServe(response, request)
	if readErr == nil || response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("readErr=%v status=%d", readErr, response.Code)
	}
}

func TestProxyHandlerDelegatesContributionRoute(t *testing.T) {
	called := false
	handler := &proxyHandler{
		cfg:              &config{},
		dataAPI:          &DataAPI{},
		providerAdminAPI: &ProviderAdminAPI{},
		providerContributionAPI: &ProviderContributionAPI{
			authorizeSession: func(http.ResponseWriter, *http.Request) bool { return true },
			handlers: map[string]http.HandlerFunc{
				"/api/pool/accounts/deepseek/add": func(w http.ResponseWriter, _ *http.Request) {
					called = true
					w.WriteHeader(http.StatusNoContent)
				},
			},
		},
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/pool/accounts/deepseek/add", nil))
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, response.Code)
	}
}
