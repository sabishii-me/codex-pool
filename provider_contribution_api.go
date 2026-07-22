package main

import "net/http"

const providerContributionBodyLimit = 2 << 20

// ProviderContributionAPI owns signed-in member routes that add new upstream
// capacity. Contributors may create connections but cannot inspect raw
// credentials, remove connections, or mutate existing lifecycle state.
type ProviderContributionAPI struct {
	authorizeSession func(http.ResponseWriter, *http.Request) bool
	handlers         map[string]http.HandlerFunc
}

func (api *ProviderContributionAPI) TryServe(w http.ResponseWriter, r *http.Request) bool {
	if api == nil || r == nil || !isProviderContributionPath(r.URL.Path) {
		return false
	}
	if !api.authorizeSession(w, r) {
		return true
	}
	if !requireMethod(w, r, http.MethodPost) {
		return true
	}
	r.Body = http.MaxBytesReader(w, r.Body, providerContributionBodyLimit)
	handler := api.handlers[r.URL.Path]
	if handler == nil {
		http.NotFound(w, r)
		return true
	}
	handler(w, r)
	return true
}

func isProviderContributionPath(path string) bool {
	const prefix = "/api/pool/accounts/"
	return len(path) > len(prefix) && path[:len(prefix)] == prefix
}

func (h *proxyHandler) providerContributionAPIService() *ProviderContributionAPI {
	if h.providerContributionAPI != nil {
		return h.providerContributionAPI
	}
	return &ProviderContributionAPI{
		authorizeSession: h.checkAdminOrSessionAuth,
		handlers: map[string]http.HandlerFunc{
			"/api/pool/accounts/codex/add":            h.handleCodexAdd,
			"/api/pool/accounts/codex/exchange":       h.handleCodexExchange,
			"/api/pool/accounts/codex/status":         h.handleCodexStatus,
			"/api/pool/accounts/claude/add":           h.handleClaudeAdd,
			"/api/pool/accounts/claude/exchange":      h.handleClaudeExchange,
			"/api/pool/accounts/antigravity/add":      h.handleAntigravityAdd,
			"/api/pool/accounts/antigravity/status":   h.handleAntigravityStatus,
			"/api/pool/accounts/antigravity/exchange": h.handleAntigravityExchange,
			"/api/pool/accounts/kimi/add":             h.handleKimiAdd,
			"/api/pool/accounts/kimi-platform/add":    h.handleKimiPlatformAdd,
			"/api/pool/accounts/minimax/add":          h.handleMinimaxAdd,
			"/api/pool/accounts/zai/add":              h.handleZAIAdd,
			"/api/pool/accounts/xiaomi/add":           h.handleXiaomiAdd,
			"/api/pool/accounts/grok/add":             h.handleGrokImport,
			"/api/pool/accounts/deepseek/add":         h.handleDeepSeekAdd,
			"/api/pool/accounts/qwen/add":             h.handleQwenAdd,
			"/api/pool/accounts/openrouter/add":       h.handleOpenRouterAdd,
			"/api/pool/accounts/nvidia/add":           h.handleNvidiaAdd,
		},
	}
}
