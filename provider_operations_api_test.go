package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderOperationsAPIPublicCallbacksBypassAdminOnly(t *testing.T) {
	adminCalls := 0
	claudeCalls := 0
	antigravityCalls := 0
	api := &ProviderOperationsAPI{
		authorizeAdmin:      func(http.ResponseWriter, *http.Request) bool { adminCalls++; return false },
		claude:              func(w http.ResponseWriter, _ *http.Request) { claudeCalls++; w.WriteHeader(http.StatusNoContent) },
		antigravityCallback: func(w http.ResponseWriter, _ *http.Request) { antigravityCalls++; w.WriteHeader(http.StatusNoContent) },
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/admin/claude/callback", nil)) || response.Code != http.StatusNoContent {
		t.Fatalf("Claude callback status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/admin/antigravity/callback", nil)) || response.Code != http.StatusNoContent {
		t.Fatalf("Antigravity callback status=%d", response.Code)
	}
	if adminCalls != 0 || claudeCalls != 1 || antigravityCalls != 1 {
		t.Fatalf("admin=%d claude=%d antigravity=%d", adminCalls, claudeCalls, antigravityCalls)
	}
}

func TestProviderOperationsAPIAntigravityCallbackRequiresGet(t *testing.T) {
	called := false
	api := &ProviderOperationsAPI{antigravityCallback: func(http.ResponseWriter, *http.Request) { called = true }}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodPost, "/admin/antigravity/callback", nil)) || response.Code != http.StatusMethodNotAllowed || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
}

func TestProviderOperationsAPIRequiresAdminForProviderDispatch(t *testing.T) {
	called := false
	api := &ProviderOperationsAPI{
		authorizeAdmin: func(w http.ResponseWriter, _ *http.Request) bool {
			http.Error(w, "not an admin", http.StatusForbidden)
			return false
		},
		codex: func(http.ResponseWriter, *http.Request) { called = true },
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodPost, "/admin/codex/add", nil)) || response.Code != http.StatusForbidden || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
}

func TestProviderOperationsAPIPreservesKimiPlatformPrefixOrdering(t *testing.T) {
	called := ""
	api := &ProviderOperationsAPI{
		authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
		providerHandlers: []providerOperationHandler{
			{prefix: "/admin/kimi-platform", handler: func(http.ResponseWriter, *http.Request) { called = "platform" }},
			{prefix: "/admin/kimi", handler: func(http.ResponseWriter, *http.Request) { called = "kimi" }},
		},
	}
	if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", nil)) || called != "platform" {
		t.Fatalf("called=%q", called)
	}
}

func TestProviderOperationsAPIRoutesAntigravityModels(t *testing.T) {
	called := ""
	api := &ProviderOperationsAPI{
		authorizeAdmin:    func(http.ResponseWriter, *http.Request) bool { return true },
		antigravitySync:   func(http.ResponseWriter, *http.Request) { called = "sync" },
		antigravityVerify: func(http.ResponseWriter, *http.Request) { called = "verify" },
	}
	for _, test := range []struct{ path, want string }{
		{"/admin/antigravity/models/sync", "sync"},
		{"/admin/antigravity/models/verify", "verify"},
	} {
		called = ""
		if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, test.path, nil)) || called != test.want {
			t.Fatalf("path=%s called=%q", test.path, called)
		}
	}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodGet, "/admin/antigravity/models/sync", nil)) || response.Code != http.StatusNotFound {
		t.Fatalf("wrong method status=%d", response.Code)
	}
}

func TestProviderOperationsAPIDoesNotClaimOtherBoundaries(t *testing.T) {
	api := &ProviderOperationsAPI{}
	for _, path := range []string{"/admin/accounts/c1/disable", "/admin/pool-users", "/api/pool/accounts/codex/add", "/v1/messages"} {
		if api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil)) {
			t.Fatalf("claimed unrelated path %s", path)
		}
	}
}

func TestProxyHandlerDelegatesProviderOperation(t *testing.T) {
	called := false
	handler := &proxyHandler{
		cfg:                     &config{},
		dataAPI:                 &DataAPI{},
		providerAdminAPI:        &ProviderAdminAPI{},
		providerContributionAPI: &ProviderContributionAPI{},
		providerOperationsAPI: &ProviderOperationsAPI{
			authorizeAdmin: func(http.ResponseWriter, *http.Request) bool { return true },
			codex:          func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) },
		},
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/admin/codex/add", nil))
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, response.Code)
	}
}
