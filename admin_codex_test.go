package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func resetCodexOAuthSessions(t *testing.T) {
	t.Helper()
	codexOAuthSessions.Lock()
	previous := codexOAuthSessions.sessions
	codexOAuthSessions.sessions = make(map[string]*CodexOAuthSession)
	codexOAuthSessions.Unlock()
	t.Cleanup(func() {
		codexOAuthSessions.Lock()
		codexOAuthSessions.sessions = previous
		codexOAuthSessions.Unlock()
	})
}

func TestCodexOAuthRedirectUsesExistingLoopbackGatewayPort(t *testing.T) {
	t.Setenv("CODEX_OAUTH_REDIRECT_URI", "")
	t.Setenv("PUBLIC_URL", "http://127.0.0.1:18990")

	h := &proxyHandler{}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18990/api/pool/accounts/codex/add", nil)
	if got := codexOAuthRedirectURI(h, req); got != "http://localhost:18990/auth/callback" {
		t.Fatalf("redirect URI = %q", got)
	}
}

func TestCodexOAuthRedirectKeepsLegacyFallbackForRemoteGateway(t *testing.T) {
	t.Setenv("CODEX_OAUTH_REDIRECT_URI", "")
	t.Setenv("PUBLIC_URL", "https://pool.example.test")

	h := &proxyHandler{}
	req := httptest.NewRequest(http.MethodPost, "https://pool.example.test/api/pool/accounts/codex/add", nil)
	if got := codexOAuthRedirectURI(h, req); got != CodexOAuthRedirectURI {
		t.Fatalf("redirect URI = %q, want remote/manual fallback %q", got, CodexOAuthRedirectURI)
	}
}

func TestCodexOAuthRedirectAllowsExplicitOverride(t *testing.T) {
	t.Setenv("CODEX_OAUTH_REDIRECT_URI", "http://localhost:4567/auth/callback")
	h := &proxyHandler{}
	req := httptest.NewRequest(http.MethodPost, "https://pool.example.test/api/pool/accounts/codex/add", nil)
	if got := codexOAuthRedirectURI(h, req); got != "http://localhost:4567/auth/callback" {
		t.Fatalf("redirect URI = %q", got)
	}
}

func TestHandleCodexAddReturnsPollableSessionAndDynamicRedirect(t *testing.T) {
	resetCodexOAuthSessions(t)
	t.Setenv("CODEX_OAUTH_REDIRECT_URI", "")
	t.Setenv("PUBLIC_URL", "http://localhost:8989")

	h := &proxyHandler{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8989/api/pool/accounts/codex/add", strings.NewReader("{}"))
	req.Header.Set("Origin", "http://localhost:8989")
	response := httptest.NewRecorder()
	h.handleCodexAdd(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		OAuthURL          string `json:"oauth_url"`
		Verifier          string `json:"verifier"`
		SessionID         string `json:"session_id"`
		Status            string `json:"status"`
		RedirectURI       string `json:"redirect_uri"`
		AutomaticCallback bool   `json:"automatic_callback"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.SessionID == "" || result.SessionID != result.Verifier || result.Status != "pending" {
		t.Fatalf("unexpected OAuth session response: %+v", result)
	}
	if result.RedirectURI != "http://localhost:8989/auth/callback" || !result.AutomaticCallback {
		t.Fatalf("redirect URI = %q, automatic = %v", result.RedirectURI, result.AutomaticCallback)
	}
	authorize, err := url.Parse(result.OAuthURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := authorize.Query().Get("redirect_uri"); got != result.RedirectURI {
		t.Fatalf("authorize redirect = %q", got)
	}
}

func TestCodexCallbackRejectsMissingCodeAndUpdatesStatus(t *testing.T) {
	resetCodexOAuthSessions(t)
	session := &CodexOAuthSession{
		Verifier:     "verifier",
		State:        "state",
		RedirectURI:  "http://localhost:8989/auth/callback",
		TargetOrigin: "http://localhost:8989",
		Status:       "pending",
		CreatedAt:    time.Now(),
	}
	codexOAuthSessions.Lock()
	codexOAuthSessions.sessions[session.Verifier] = session
	codexOAuthSessions.Unlock()

	h := &proxyHandler{}
	response := httptest.NewRecorder()
	h.handleCodexCallback(response, httptest.NewRequest(http.MethodGet, "http://localhost:8989/auth/callback?state=state", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "error") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	codexOAuthSessions.RLock()
	status, message := session.Status, session.Error
	codexOAuthSessions.RUnlock()
	if status != "error" || message != "authorization code is missing" {
		t.Fatalf("session status = %q, error = %q", status, message)
	}
}
