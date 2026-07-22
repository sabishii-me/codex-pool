package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticationAPIRoutesExactHandlers(t *testing.T) {
	called := ""
	api := &AuthenticationAPI{
		googleLogin:    func(http.ResponseWriter, *http.Request) { called = "login" },
		googleCallback: func(http.ResponseWriter, *http.Request) { called = "google-callback" },
		codexCallback:  func(http.ResponseWriter, *http.Request) { called = "codex-callback" },
		logout:         func(http.ResponseWriter, *http.Request) { called = "logout" },
		session:        func(http.ResponseWriter, *http.Request) { called = "session" },
		mfaHandlers: map[string]http.HandlerFunc{
			"/api/admin/mfa/status": func(http.ResponseWriter, *http.Request) { called = "mfa" },
		},
	}
	cases := []struct{ method, path, want string }{
		{http.MethodGet, "/auth/login/google", "login"},
		{http.MethodGet, "/auth/callback/google", "google-callback"},
		{http.MethodGet, "/auth/callback/codex", "codex-callback"},
		{http.MethodPost, "/auth/logout", "logout"},
		{http.MethodGet, "/api/pool/session", "session"},
		{http.MethodGet, "/api/admin/mfa/status", "mfa"},
	}
	for _, test := range cases {
		called = ""
		if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(test.method, test.path, nil)) || called != test.want {
			t.Fatalf("path=%s called=%q", test.path, called)
		}
	}
}

func TestAuthenticationAPICodexCallbackRequiresGet(t *testing.T) {
	called := false
	api := &AuthenticationAPI{codexCallback: func(http.ResponseWriter, *http.Request) { called = true }}
	response := httptest.NewRecorder()
	if !api.TryServe(response, httptest.NewRequest(http.MethodPost, "/auth/callback/codex", nil)) || response.Code != http.StatusMethodNotAllowed || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
}

func TestAuthenticationAPIDoesNotClaimOtherBoundaries(t *testing.T) {
	api := &AuthenticationAPI{}
	for _, path := range []string{"/", "/admin/codex", "/api/pool/accounts/codex/add", "/v1/messages", "/auth/unknown", "/api/admin/mfa/unknown"} {
		if api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil)) {
			t.Fatalf("claimed unrelated path %s", path)
		}
	}
}

func TestProxyHandlerDelegatesAuthenticationRoute(t *testing.T) {
	called := false
	handler := &proxyHandler{
		cfg:                     &config{},
		dataAPI:                 &DataAPI{},
		providerAdminAPI:        &ProviderAdminAPI{},
		providerContributionAPI: &ProviderContributionAPI{},
		providerOperationsAPI:   &ProviderOperationsAPI{},
		authenticationAPI: &AuthenticationAPI{
			session: func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) },
		},
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/pool/session", nil))
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, response.Code)
	}
}
