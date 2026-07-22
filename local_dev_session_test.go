package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalDevSessionURLRequiresLoopback(t *testing.T) {
	for _, allowed := range []string{
		"http://127.0.0.1:18990", "http://localhost:18990", "http://[::1]:18990",
	} {
		if !localDevSessionURLAllowed(allowed) {
			t.Errorf("loopback URL rejected: %s", allowed)
		}
	}
	for _, denied := range []string{
		"", "https://pool.example.com", "http://0.0.0.0:18990", "http://192.168.1.10:18990",
	} {
		if localDevSessionURLAllowed(denied) {
			t.Errorf("non-loopback URL allowed: %s", denied)
		}
	}
}

func TestLocalDevSessionRequiresLoopbackRequestHost(t *testing.T) {
	store, err := newGatewayUserStore(filepath.Join(t.TempDir(), "pool_users.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureLocalDevelopmentUser(store); err != nil {
		t.Fatal(err)
	}
	handler := &proxyHandler{cfg: &config{localDevSession: true}, poolUsers: store}
	for _, rawURL := range []string{"http://127.0.0.1:18990/api/pool/session", "http://localhost:18990/api/pool/session"} {
		request := httptest.NewRequest(http.MethodGet, rawURL, nil)
		request.RemoteAddr = "127.0.0.1:54321"
		if user, ok := handler.sessionUser(request); !ok || user.ID != localDevelopmentUserID {
			t.Fatalf("loopback request did not resolve local user: %s", rawURL)
		}
	}
	publicRequest := httptest.NewRequest(http.MethodGet, "https://pool.example.com/api/pool/session", nil)
	publicRequest.RemoteAddr = "203.0.113.10:54321"
	if _, ok := handler.sessionUser(publicRequest); ok {
		t.Fatal("public request host resolved local development user")
	}
	spoofedHost := httptest.NewRequest(http.MethodGet, "http://localhost:18990/api/pool/session", nil)
	spoofedHost.RemoteAddr = "203.0.113.10:54321"
	if _, ok := handler.sessionUser(spoofedHost); ok {
		t.Fatal("non-loopback peer spoofed a loopback Host")
	}
}

func TestLocalDevModeServesCurrentReactShellWithoutOAuth(t *testing.T) {
	handler := &proxyHandler{cfg: &config{localDevSession: true}}
	response := httptest.NewRecorder()
	handler.serveFriendLanding(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18990/", nil))
	body := response.Body.String()
	for _, expected := range []string{`<div id="root"></div>`, `AI Pool — Full-Spectrum Signal Room`, `src="/assets/`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("React shell missing %q", expected)
		}
	}
	if strings.Contains(body, `id="access-form"`) {
		t.Fatal("local development mode served legacy landing page")
	}
}

func TestLocalDevModeReturnsSyntheticSessionOnlyWhenEnabled(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "isolated-development-secret")
	store, err := newGatewayUserStore(filepath.Join(t.TempDir(), "pool_users.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureLocalDevelopmentUser(store); err != nil {
		t.Fatal(err)
	}
	handler := &proxyHandler{cfg: &config{localDevSession: true}, poolUsers: store}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18990/api/pool/session", nil)
	request.RemoteAddr = "127.0.0.1:54321"
	response := httptest.NewRecorder()
	handler.handlePoolSession(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"email":"developer@localhost.invalid"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	handler.cfg.localDevSession = false
	response = httptest.NewRecorder()
	handler.handlePoolSession(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("disabled local session status=%d", response.Code)
	}
}
