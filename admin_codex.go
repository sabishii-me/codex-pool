package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Codex OAuth constants (from codex-rs/login/src/server.rs)
const (
	CodexOAuthClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexOAuthTokenURL     = "https://auth.openai.com/oauth/token"
	CodexOAuthAuthorizeURL = "https://auth.openai.com/oauth/authorize"
)

// CodexOAuthSession stores pending OAuth state
type CodexOAuthSession struct {
	AccountID    string
	Verifier     string
	Challenge    string
	State        string
	RedirectURI  string
	TargetOrigin string
	Status       string
	Error        string
	CreatedAt    time.Time
}

// In-memory store for pending Codex OAuth sessions
var codexOAuthSessions = struct {
	sync.RWMutex
	sessions map[string]*CodexOAuthSession
}{sessions: make(map[string]*CodexOAuthSession)}

// CodexTokenResponse is the response from the token endpoint
type CodexTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// serveCodexAdmin routes Codex admin requests
func (h *proxyHandler) serveCodexAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/codex")
	if path == "" {
		path = "/"
	}

	switch {
	case path == "/" && r.Method == http.MethodGet:
		h.handleCodexList(w, r)

	case path == "/add" && r.Method == http.MethodPost:
		h.handleCodexAdd(w, r)

	case path == "/exchange" && r.Method == http.MethodPost:
		h.handleCodexExchange(w, r)

	case path == "/status" && r.Method == http.MethodPost:
		h.handleCodexStatus(w, r)

	default:
		http.NotFound(w, r)
	}
}

// GET /admin/codex - list all Codex accounts
func (h *proxyHandler) handleCodexList(w http.ResponseWriter, r *http.Request) {
	accounts := h.pool.allAccounts()

	type accountInfo struct {
		ID          string    `json:"id"`
		PlanType    string    `json:"plan_type"`
		Dead        bool      `json:"dead"`
		Disabled    bool      `json:"disabled"`
		CyberAccess bool      `json:"cyber_access,omitempty"`
		ExpiresAt   time.Time `json:"expires_at,omitempty"`
		LastRefresh time.Time `json:"last_refresh,omitempty"`
	}

	var result []accountInfo
	for _, acc := range accounts {
		if acc.Type == AccountTypeCodex {
			result = append(result, accountInfo{
				ID:          acc.ID,
				PlanType:    acc.PlanType,
				Dead:        acc.Dead,
				Disabled:    acc.Disabled,
				CyberAccess: acc.CyberAccess,
				ExpiresAt:   acc.ExpiresAt,
				LastRefresh: acc.LastRefresh,
			})
		}
	}

	respondJSON(w, map[string]any{
		"accounts": result,
		"count":    len(result),
	})
}

// POST /admin/codex/add - start OAuth flow
func (h *proxyHandler) handleCodexAdd(w http.ResponseWriter, r *http.Request) {
	// Generate PKCE verifier and challenge
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to generate verifier")
		return
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	challengeHash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeHash[:])

	// Generate state
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	redirectURI := codexOAuthRedirectURI()

	// OpenAI allowlists only Codex's loopback callback ports. A separate relay
	// binds this address for one authorization and immediately releases it.
	u, _ := url.Parse(CodexOAuthAuthorizeURL)
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", CodexOAuthClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "openid profile email offline_access")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("state", state)
	q.Set("originator", "codex_cli_rs")
	u.RawQuery = q.Encode()

	// Store session
	session := &CodexOAuthSession{
		Verifier:     verifier,
		Challenge:    challenge,
		State:        state,
		RedirectURI:  redirectURI,
		TargetOrigin: codexOAuthTargetOrigin(r, h),
		Status:       "pending",
		CreatedAt:    time.Now(),
	}

	codexOAuthSessions.Lock()
	codexOAuthSessions.sessions[verifier] = session
	codexOAuthSessions.Unlock()

	// Clean up old sessions
	go cleanupOldCodexSessions()

	respondJSON(w, map[string]any{
		"oauth_url":      u.String(),
		"verifier":       verifier,
		"state":          state,
		"session_id":     verifier,
		"status":         "pending",
		"redirect_uri":   redirectURI,
		"relay_required": true,
	})
}

// POST /admin/codex/status - poll automatic callback completion.
func (h *proxyHandler) handleCodexStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	codexOAuthSessions.RLock()
	session := codexOAuthSessions.sessions[strings.TrimSpace(req.SessionID)]
	if session == nil || time.Since(session.CreatedAt) > 30*time.Minute {
		codexOAuthSessions.RUnlock()
		respondJSONError(w, http.StatusNotFound, "invalid or expired session")
		return
	}
	status, accountID, sessionError := session.Status, session.AccountID, session.Error
	codexOAuthSessions.RUnlock()
	respondJSON(w, map[string]any{
		"session_id": req.SessionID,
		"status":     status,
		"account_id": accountID,
		"error":      sessionError,
	})
}

func codexOAuthRedirectURI() string {
	port := strings.TrimSpace(os.Getenv("CODEX_OAUTH_PORT"))
	if port != "1457" {
		port = "1455"
	}
	return "http://localhost:" + port + "/auth/callback"
}

func codexOAuthTargetOrigin(r *http.Request, h *proxyHandler) string {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		if parsed, err := url.Parse(origin); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host
		}
	}
	if parsed, err := url.Parse(h.getEffectivePublicURL(r)); err == nil && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host
	}
	return "*"
}

// GET /auth/callback/codex receives the callback forwarded by the temporary
// loopback relay. It deliberately does not require a session cookie: the
// random OAuth state and PKCE verifier authenticate the callback.
func (h *proxyHandler) handleCodexCallback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	codexOAuthSessions.Lock()
	var session *CodexOAuthSession
	for _, candidate := range codexOAuthSessions.sessions {
		if candidate.State == state {
			session = candidate
			break
		}
	}
	if session == nil || time.Since(session.CreatedAt) > 30*time.Minute {
		codexOAuthSessions.Unlock()
		h.renderCodexCallback(w, nil, "error", "", "invalid or expired OAuth state")
		return
	}
	if session.Status == "complete" {
		accountID := session.AccountID
		codexOAuthSessions.Unlock()
		h.renderCodexCallback(w, session, "complete", accountID, "")
		return
	}
	if session.Status == "exchanging" {
		codexOAuthSessions.Unlock()
		h.renderCodexCallback(w, session, "exchanging", "", "")
		return
	}
	if upstreamError := strings.TrimSpace(r.URL.Query().Get("error")); upstreamError != "" {
		session.Status, session.Error = "error", upstreamError
		codexOAuthSessions.Unlock()
		h.renderCodexCallback(w, session, "error", "", upstreamError)
		return
	}
	session.Status, session.Error = "exchanging", ""
	codexOAuthSessions.Unlock()

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		h.setCodexOAuthError(session, "authorization code is missing")
		h.renderCodexCallback(w, session, "error", "", "authorization code is missing")
		return
	}
	tokens, err := codexExchangeCode(code, session.Verifier, session.RedirectURI)
	if err != nil {
		log.Printf("Codex callback token exchange failed: %v", err)
		h.setCodexOAuthError(session, "token exchange failed")
		h.renderCodexCallback(w, session, "error", "", "token exchange failed")
		return
	}
	accountID := generateCodexAccountID(tokens.IDToken)
	if err := saveNewCodexAccount(filepath.Join(h.cfg.poolDir, "codex"), accountID, tokens); err != nil {
		h.setCodexOAuthError(session, "failed to save account")
		h.renderCodexCallback(w, session, "error", "", "failed to save account")
		return
	}
	h.reloadAccounts()
	codexOAuthSessions.Lock()
	session.Status, session.AccountID, session.Error = "complete", accountID, ""
	codexOAuthSessions.Unlock()
	h.renderCodexCallback(w, session, "complete", accountID, "")
}

func (h *proxyHandler) setCodexOAuthError(session *CodexOAuthSession, message string) {
	codexOAuthSessions.Lock()
	session.Status, session.Error = "error", message
	codexOAuthSessions.Unlock()
}

func (h *proxyHandler) renderCodexCallback(w http.ResponseWriter, session *CodexOAuthSession, status, accountID, message string) {
	sessionID, targetOrigin := "", "*"
	if session != nil {
		sessionID, targetOrigin = session.Verifier, session.TargetOrigin
	}
	payload, _ := json.Marshal(map[string]string{
		"type": "codex-pool-codex-oauth", "session_id": sessionID,
		"status": status, "account_id": accountID, "error": message,
	})
	encodedPayload := base64.StdEncoding.EncodeToString(payload)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Codex sign-in</title><p>%s</p><script>const payload=JSON.parse(atob(%q));if(window.opener){window.opener.postMessage(payload,%q)}window.close()</script>`, template.HTMLEscapeString(status), encodedPayload, targetOrigin)
}

// POST /admin/codex/exchange - exchange a manually pasted OAuth code.
func (h *proxyHandler) handleCodexExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string `json:"code"`
		Verifier string `json:"verifier"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}
	} else {
		req.Code = r.FormValue("code")
		req.Verifier = r.FormValue("verifier")
	}

	code := strings.TrimSpace(req.Code)
	verifier := strings.TrimSpace(req.Verifier)

	if code == "" || verifier == "" {
		respondJSONError(w, http.StatusBadRequest, "code and verifier are required")
		return
	}

	// Look up session
	codexOAuthSessions.RLock()
	session, ok := codexOAuthSessions.sessions[verifier]
	codexOAuthSessions.RUnlock()

	if !ok {
		respondJSONError(w, http.StatusBadRequest, "invalid or expired session")
		return
	}

	// Exchange code for tokens
	tokens, err := codexExchangeCode(code, verifier, session.RedirectURI)
	if err != nil {
		log.Printf("Codex token exchange failed: %v", err)
		respondJSONError(w, http.StatusInternalServerError, "token exchange failed: "+err.Error())
		return
	}

	// Generate account ID from email in id_token
	accountID := generateCodexAccountID(tokens.IDToken)

	// Save the account
	poolDir := filepath.Join(h.cfg.poolDir, "codex")
	if err := saveNewCodexAccount(poolDir, accountID, tokens); err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to save account: "+err.Error())
		return
	}

	codexOAuthSessions.Lock()
	session.Status = "complete"
	session.AccountID = accountID
	session.Error = ""
	codexOAuthSessions.Unlock()

	// Reload accounts
	h.reloadAccounts()

	respondJSON(w, map[string]any{
		"success":    true,
		"account_id": accountID,
	})
}

// codexExchangeCode exchanges an authorization code for tokens
func codexExchangeCode(code, verifier, redirectURI string) (*CodexTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", CodexOAuthClientID)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", verifier)

	req, err := http.NewRequest(http.MethodPost, CodexOAuthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: %s: %s", resp.Status, string(body))
	}

	var tokens CodexTokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokens.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response")
	}

	return &tokens, nil
}

// generateCodexAccountID derives a stable, non-identifying filename from the
// upstream ChatGPT account ID. Email-derived names collide for aliases and
// multi-workspace users; the full SHA-256 keeps each upstream account distinct.
func generateCodexAccountID(idToken string) string {
	identity := strings.TrimSpace(parseCodexClaims(idToken).ChatGPTAccountID)
	if identity == "" {
		identity = codexTokenEmail(idToken)
	}
	if identity == "" {
		// An ID token should always contain an account ID or email. Hashing the
		// token is a privacy-preserving last resort rather than exposing it.
		identity = idToken
	}
	hash := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(hash[:])
}

func codexTokenEmail(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return ""
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return ""
	}
	if profile, ok := payload["https://api.openai.com/profile"].(map[string]any); ok {
		if email, ok := profile["email"].(string); ok {
			return strings.ToLower(strings.TrimSpace(email))
		}
	}
	if email, ok := payload["email"].(string); ok {
		return strings.ToLower(strings.TrimSpace(email))
	}
	return ""
}

// saveNewCodexAccount upserts a Codex connection by its stable account hash.
// Reauthorization replaces tokens without duplicating the same upstream
// account or discarding durable connection metadata.
func saveNewCodexAccount(poolDir, accountID string, tokens *CodexTokenResponse) error {
	if err := os.MkdirAll(poolDir, 0755); err != nil {
		return fmt.Errorf("create pool dir: %w", err)
	}
	filePath := filepath.Join(poolDir, accountID+".json")
	authJSON := map[string]any{}
	if existing, err := os.ReadFile(filePath); err == nil {
		if err := json.Unmarshal(existing, &authJSON); err != nil {
			return fmt.Errorf("parse existing account: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read existing account: %w", err)
	}
	if _, exists := authJSON["added_at"]; !exists {
		authJSON["added_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	authJSON["tokens"] = map[string]any{
		"id_token":      tokens.IDToken,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	}

	data, err := json.MarshalIndent(authJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		return fmt.Errorf("secure account file: %w", err)
	}
	log.Printf("Saved Codex account: %s -> %s", accountID, filePath)
	return nil
}

func cleanupOldCodexSessions() {
	codexOAuthSessions.Lock()
	defer codexOAuthSessions.Unlock()

	now := time.Now()
	for verifier, session := range codexOAuthSessions.sessions {
		if now.Sub(session.CreatedAt) > 30*time.Minute {
			delete(codexOAuthSessions.sessions, verifier)
		}
	}
}
