package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resetCodexOAuthSessions(t *testing.T) {
	t.Helper()
	codexOAuthSessions.Lock()
	previous := codexOAuthSessions.sessions
	previousPath := codexOAuthSessions.path
	codexOAuthSessions.sessions = make(map[string]*CodexOAuthSession)
	codexOAuthSessions.path = ""
	codexOAuthSessions.Unlock()
	t.Cleanup(func() {
		codexOAuthSessions.Lock()
		codexOAuthSessions.sessions = previous
		codexOAuthSessions.path = previousPath
		codexOAuthSessions.Unlock()
	})
}

func codexTestIDToken(t *testing.T, accountID, email string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"email":                       email,
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID, "chatgpt_plan_type": "plus"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func TestGenerateCodexAccountIDUsesFullStableAccountHash(t *testing.T) {
	first := generateCodexAccountID(codexTestIDToken(t, "account-one", "same@example.com"))
	sameAccount := generateCodexAccountID(codexTestIDToken(t, "account-one", "changed@example.com"))
	second := generateCodexAccountID(codexTestIDToken(t, "account-two", "same@example.com"))
	wantHash := sha256.Sum256([]byte("account-one"))
	want := hex.EncodeToString(wantHash[:])
	if first != want || len(first) != 64 {
		t.Fatalf("account hash = %q, want %q", first, want)
	}
	if sameAccount != first {
		t.Fatalf("same upstream account changed hash: %q != %q", sameAccount, first)
	}
	if second == first {
		t.Fatalf("different upstream accounts shared hash %q", first)
	}
}

func TestSaveNewCodexAccountUpsertsStableFileAndPreservesMetadata(t *testing.T) {
	dir := t.TempDir()
	accountID := generateCodexAccountID(codexTestIDToken(t, "account-one", "person@example.com"))
	first := &CodexTokenResponse{IDToken: codexTestIDToken(t, "account-one", "person@example.com"), AccessToken: "first", RefreshToken: "refresh-one"}
	if err := saveNewCodexAccount(dir, accountID, first); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, accountID+".json")
	var stored map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	addedAt := stored["added_at"]
	stored["disabled"] = true
	data, _ = json.Marshal(stored)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	second := &CodexTokenResponse{IDToken: first.IDToken, AccessToken: "second", RefreshToken: "refresh-two"}
	if err := saveNewCodexAccount(dir, accountID, second); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != accountID+".json" {
		t.Fatalf("credential files = %v", entries)
	}
	data, _ = os.ReadFile(path)
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	tokens := stored["tokens"].(map[string]any)
	if tokens["access_token"] != "second" || stored["disabled"] != true || stored["added_at"] != addedAt {
		t.Fatalf("upserted credential = %#v", stored)
	}
}

func TestCodexOAuthRedirectUsesBrokerSelectedPort(t *testing.T) {
	if got := codexOAuthRedirectURI(1455); got != "http://localhost:1455/auth/callback" {
		t.Fatalf("1455 redirect URI = %q", got)
	}
	if got := codexOAuthRedirectURI(1457); got != "http://localhost:1457/auth/callback" {
		t.Fatalf("1457 redirect URI = %q", got)
	}
}

func TestHandleCodexAddRejectsUnapprovedRedirectPort(t *testing.T) {
	resetCodexOAuthSessions(t)
	request := httptest.NewRequest(http.MethodPost, "/api/pool/accounts/codex/add", strings.NewReader(`{"redirect_port":8999}`))
	response := httptest.NewRecorder()
	(&proxyHandler{}).handleCodexAdd(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "1455 or 1457") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHandleCodexAddReturnsPollableRelaySession(t *testing.T) {
	resetCodexOAuthSessions(t)
	h := &proxyHandler{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8989/api/pool/accounts/codex/add", strings.NewReader(`{"redirect_port":1457}`))
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

func TestCodexOAuthSessionSurvivesStoreReload(t *testing.T) {
	resetCodexOAuthSessions(t)
	path := filepath.Join(t.TempDir(), "codex_oauth_sessions.json")
	if err := configureCodexOAuthSessions(path); err != nil {
		t.Fatal(err)
	}
	h := &proxyHandler{}
	request := httptest.NewRequest(http.MethodPost, "/api/pool/accounts/codex/add", strings.NewReader(`{"redirect_port":1455}`))
	request.Header.Set("Origin", "http://localhost:8989")
	response := httptest.NewRecorder()
	h.handleCodexAdd(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", response.Code, response.Body.String())
	}
	var started struct {
		SessionID string `json:"session_id"`
		State     string `json:"state"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	codexOAuthSessions.Lock()
	codexOAuthSessions.sessions = map[string]*CodexOAuthSession{}
	codexOAuthSessions.Unlock()
	if err := configureCodexOAuthSessions(path); err != nil {
		t.Fatal(err)
	}
	codexOAuthSessions.RLock()
	restored := codexOAuthSessions.sessions[started.SessionID]
	codexOAuthSessions.RUnlock()
	if restored == nil || restored.State != started.State || restored.Verifier == "" || restored.RedirectURI != "http://localhost:1455/auth/callback" {
		t.Fatalf("restored session = %#v", restored)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "authorization_code") || strings.Contains(string(data), `"code"`) {
		t.Fatalf("session store persisted an authorization code: %s", data)
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
