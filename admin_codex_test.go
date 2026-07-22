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

func TestCodexOAuthRedirectUsesAllowlistedTemporaryRelayPort(t *testing.T) {
	t.Setenv("CODEX_OAUTH_PORT", "")
	if got := codexOAuthRedirectURI(); got != "http://localhost:1455/auth/callback" {
		t.Fatalf("default redirect URI = %q", got)
	}
	t.Setenv("CODEX_OAUTH_PORT", "1457")
	if got := codexOAuthRedirectURI(); got != "http://localhost:1457/auth/callback" {
		t.Fatalf("fallback redirect URI = %q", got)
	}
	t.Setenv("CODEX_OAUTH_PORT", "9999")
	if got := codexOAuthRedirectURI(); got != "http://localhost:1455/auth/callback" {
		t.Fatalf("invalid-port redirect URI = %q", got)
	}
}

func TestHandleCodexAddReturnsPollableRelaySession(t *testing.T) {
	resetCodexOAuthSessions(t)
	t.Setenv("CODEX_OAUTH_PORT", "1457")
	h := &proxyHandler{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8989/api/pool/accounts/codex/add", strings.NewReader("{}"))
	req.Header.Set("Origin", "http://localhost:8989")
	response := httptest.NewRecorder()
	h.handleCodexAdd(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		OAuthURL      string `json:"oauth_url"`
		Verifier      string `json:"verifier"`
		SessionID     string `json:"session_id"`
		Status        string `json:"status"`
		RedirectURI   string `json:"redirect_uri"`
		RelayRequired bool   `json:"relay_required"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.SessionID == "" || result.SessionID != result.Verifier || result.Status != "pending" || !result.RelayRequired {
		t.Fatalf("unexpected OAuth session response: %+v", result)
	}
	if result.RedirectURI != "http://localhost:1457/auth/callback" {
		t.Fatalf("redirect URI = %q", result.RedirectURI)
	}
	authorize, err := url.Parse(result.OAuthURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := authorize.Query().Get("redirect_uri"); got != result.RedirectURI {
		t.Fatalf("authorize redirect = %q", got)
	}
}

func TestCodexGatewayCallbackRejectsMissingCodeAndUpdatesStatus(t *testing.T) {
	resetCodexOAuthSessions(t)
	session := &CodexOAuthSession{
		Verifier:     "verifier",
		State:        "state",
		RedirectURI:  "http://localhost:1455/auth/callback",
		TargetOrigin: "http://localhost:8989",
		Status:       "pending",
		CreatedAt:    time.Now(),
	}
	codexOAuthSessions.Lock()
	codexOAuthSessions.sessions[session.Verifier] = session
	codexOAuthSessions.Unlock()

	response := httptest.NewRecorder()
	(&proxyHandler{}).handleCodexCallback(response, httptest.NewRequest(http.MethodGet, "http://localhost:8989/auth/callback/codex?state=state", nil))
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
