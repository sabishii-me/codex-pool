package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

func normalizeNoopPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	// Reverse proxies and clients sometimes leave a trailing slash.
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path
}

// isCodexAppsMCPPath matches the streamable-HTTP endpoint Codex uses for the
// built-in codex_apps MCP server. Path can arrive with or without the
// /backend-api prefix depending on how the public reverse proxy rewrites.
func isCodexAppsMCPPath(path string) bool {
	path = normalizeNoopPath(path)
	switch path {
	case "/api/codex/apps",
		"/backend-api/wham/apps",
		"/wham/apps",
		"/apps",
		"/backend-api/ps/mcp",
		"/api/codex/ps/mcp",
		"/ps/mcp":
		return true
	}
	return strings.HasSuffix(path, "/wham/apps") ||
		strings.HasSuffix(path, "/codex/apps") ||
		strings.HasSuffix(path, "/ps/mcp")
}

func isCodexResetCreditsPath(path string) bool {
	path = normalizeNoopPath(path)
	return strings.HasSuffix(path, "/rate-limit-reset-credits") ||
		strings.HasSuffix(path, "/rate-limit-reset-credits/consume")
}

func servePoolCodexResetCredits(w http.ResponseWriter, r *http.Request) {
	path := normalizeNoopPath(r.URL.Path)
	if strings.HasSuffix(path, "/consume") {
		respondJSON(w, map[string]any{
			"code":          "no_credit",
			"windows_reset": 0,
		})
		return
	}
	respondJSON(w, map[string]any{
		"available_count": 0,
		"credits":         []any{},
	})
}

func shouldNoopCodexPath(path string) bool {
	path = normalizeNoopPath(path)
	if isCodexAppsMCPPath(path) {
		return true
	}
	// OAuth discovery for streamable HTTP is rooted at the MCP URL path.
	if strings.HasPrefix(path, "/.well-known/oauth-authorization-server") {
		return true
	}
	switch path {
	case "/connectors/directory/list",
		"/connectors/directory/list_workspace",
		"/codex/analytics-events/events",
		"/v1/traces/ingest",
		"/plugins/featured",
		"/plugins/list",
		"/backend-api/plugins/featured",
		"/backend-api/codex/analytics-events/events":
		return true
	default:
		return false
	}
}

func serveNoopCodexPath(w http.ResponseWriter, r *http.Request) {
	path := normalizeNoopPath(r.URL.Path)
	if isCodexAppsMCPPath(path) {
		serveNoopCodexAppsMCP(w, r)
		return
	}
	if strings.HasPrefix(path, "/.well-known/oauth-authorization-server") {
		// Empty discovery doc: codex_apps does not need OAuth through the pool.
		respondJSON(w, map[string]any{
			"authorization_endpoint": "",
			"token_endpoint":         "",
			"scopes_supported":       []string{},
		})
		return
	}
	switch path {
	case "/connectors/directory/list", "/connectors/directory/list_workspace":
		respondJSON(w, map[string]any{
			"apps":      []any{},
			"nextToken": nil,
		})
	case "/v1/traces/ingest":
		respondJSON(w, map[string]any{"ok": true})
	default:
		// Empty success for noisy telemetry / plugin list probes.
		w.WriteHeader(http.StatusOK)
	}
}

func serveNoopCodexAppsMCP(w http.ResponseWriter, r *http.Request) {
	// Streamable HTTP may open with GET (SSE) or OPTIONS; neither needs tools.
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	_ = json.Unmarshal(body, &req)

	// Notifications (no id) — accept and stop. Includes notifications/initialized.
	if len(req.ID) == 0 || string(req.ID) == "null" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		return
	}

	result := map[string]any{}
	switch req.Method {
	case "initialize":
		// Echo the client's protocolVersion when present; clients reject
		// unsupported versions. Empty tools is intentional — pool has no apps.
		protocolVersion := "2025-06-18"
		if len(req.Params) > 0 {
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if err := json.Unmarshal(req.Params, &params); err == nil && strings.TrimSpace(params.ProtocolVersion) != "" {
				protocolVersion = strings.TrimSpace(params.ProtocolVersion)
			}
		}
		result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{"name": "codex_apps", "version": "0.0.0"},
		}
	case "tools/list":
		// Empty tool list: quiet success instead of 401/handshake failure.
		result = map[string]any{"tools": []any{}}
	case "resources/list":
		result = map[string]any{"resources": []any{}}
	case "resources/templates/list":
		result = map[string]any{"resourceTemplates": []any{}}
	case "prompts/list":
		result = map[string]any{"prompts": []any{}}
	case "ping":
		result = map[string]any{}
	default:
		// Unknown methods: empty result rather than hard error so startup
		// probes do not surface as MCP client failures.
		result = map[string]any{}
	}

	respondJSON(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(req.ID),
		"result":  result,
	})
}

func isFrontendNavigationPath(path string) bool {
	path = normalizeNoopPath(path)
	switch path {
	case "/", "/models", "/usage", "/setup", "/profile",
		"/admin/connections", "/admin/members", "/admin/system":
		return true
	}
	// Discarded development routes still receive the SPA shell so React can
	// render its explicit not-found page. They are not aliases or redirects.
	return strings.HasPrefix(path, "/operator") ||
		path == "/admin/routes" || path == "/admin/usage" || path == "/admin/monitor"
}

func isFrontendNavigationRequest(r *http.Request) bool {
	if r == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return false
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html") {
		return false
	}
	return isFrontendNavigationPath(r.URL.Path)
}

// ServeHTTP routes incoming requests to the appropriate handler.
func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqID := randomID()
	if h.cfg.debug.Load() {
		log.Printf("[%s] incoming %s %s", reqID, r.Method, r.URL.Path)
	}

	// Fingerprinted frontend assets are embedded by the Go binary.
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		h.serveSignalRoomAsset(w, r)
		return
	}

	// Read-only data routes are owned separately from authentication,
	// administration mutations, and gateway proxy execution.
	if h.dataAPIService().TryServe(w, r) {
		return
	}
	if r.URL.Path == "/api/v2/setup/clients" {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		h.handleSetupClients(w, r)
		return
	}
	if h.providerAdminAPIService().TryServe(w, r) {
		return
	}
	if h.providerContributionAPIService().TryServe(w, r) {
		return
	}
	if h.providerOperationsAPIService().TryServe(w, r) {
		return
	}
	if h.authenticationAPIService().TryServe(w, r) {
		return
	}
	if h.systemAdminAPIService().TryServe(w, r) {
		return
	}
	// Cute Code was removed rather than retained as a compatibility surface.
	if r.URL.Path == "/cute-code" || strings.HasPrefix(r.URL.Path, "/setup/cute-code/") || strings.HasPrefix(r.URL.Path, "/config/cute-code/") {
		http.NotFound(w, r)
		return
	}

	// Browser navigation is a separate boundary from API and model-proxy
	// routing. Canonical product routes serve the embedded SPA directly,
	// including signed-out deep links and page reloads.
	if isFrontendNavigationRequest(r) {
		h.serveFriendLanding(w, r)
		return
	}

	// Static routes
	switch r.URL.Path {
	case "/":
		h.serveFriendLanding(w, r)
		return
	case "/status":
		h.serveStatusPage(w, r)
		return
	case "/og-image.png":
		h.serveOGImage(w, r)
		return
	case "/hero.png", "/hero.webp":
		h.serveHeroImage(w, r)
		return
	case "/favicon.ico":
		http.NotFound(w, r)
		return
	case "/healthz":
		h.serveHealth(w)
		return
	}

	// Friend landing page with code
	if strings.HasPrefix(r.URL.Path, "/friend/") {
		h.serveFriendLanding(w, r)
		return
	}

	// Setup scripts
	if strings.HasPrefix(r.URL.Path, "/setup/codex/") {
		h.serveCodexSetupScript(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/setup/gemini/") {
		h.serveGeminiSetupScript(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/setup/claude/") {
		h.serveClaudeSetupScript(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/setup/grok/") {
		h.serveGrokSetupScript(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/setup/pi/") {
		h.servePiSetupScript(w, r)
		return
	}
	// Config download routes (no auth - token is the auth)
	if strings.HasPrefix(r.URL.Path, "/config/codex/") || strings.HasPrefix(r.URL.Path, "/config/gemini/") || strings.HasPrefix(r.URL.Path, "/config/claude/") || strings.HasPrefix(r.URL.Path, "/config/pi/") || strings.HasPrefix(r.URL.Path, "/config/grok/") {
		h.serveConfigDownload(w, r)
		return
	}

	// Fake refresh handler so Codex CLI never needs to hit the real auth server.
	if strings.HasPrefix(r.URL.Path, "/oauth/token") {
		h.serveFakeOAuthToken(w, r)
		return
	}

	// Pool users never own reset credits. Redemption is reserved for the
	// server-side account poller, which calls ChatGPT directly.
	if isCodexResetCreditsPath(r.URL.Path) {
		servePoolCodexResetCredits(w, r)
		return
	}

	// Special case: aggregate usage for client; do not hit upstream.
	if isUsageRequest(r) {
		h.pollUpstreamUsage()
		h.handleAggregatedUsage(w, reqID)
		return
	}

	// Claude-specific endpoints - return pool info instead of individual account info
	if isClaudeProfileRequest(r) {
		h.handleClaudeProfile(w, r)
		return
	}
	if isClaudeUsageRequest(r) {
		h.handleClaudeUsage(w, r)
		return
	}

	if shouldNoopCodexPath(r.URL.Path) {
		serveNoopCodexPath(w, r)
		return
	}

	// Default: proxy to upstream
	h.proxyRequest(w, r, reqID)
}
