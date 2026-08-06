package main

import (
	"net/http"
	"strings"
)

// ProviderOperationsAPI owns elevated provider-specific key, OAuth, discovery,
// and removal routes. Credential contribution by ordinary signed-in members is
// owned separately by ProviderContributionAPI.
type ProviderOperationsAPI struct {
	authorizeAdmin func(http.ResponseWriter, *http.Request) bool

	codex               http.HandlerFunc
	antigravityCallback http.HandlerFunc
	antigravitySync     http.HandlerFunc
	antigravityVerify   http.HandlerFunc
	providerHandlers    []providerOperationHandler
}

type providerOperationHandler struct {
	prefix  string
	handler http.HandlerFunc
}

func (api *ProviderOperationsAPI) TryServe(w http.ResponseWriter, r *http.Request) bool {
	if api == nil || r == nil {
		return false
	}

	// OAuth callbacks are authenticated by upstream state/PKCE rather than an
	// operator cookie because browsers reach them through provider redirects.
	if r.URL.Path == "/admin/antigravity/callback" {
		if !requireMethod(w, r, http.MethodGet) {
			return true
		}
		api.antigravityCallback(w, r)
		return true
	}

	if strings.HasPrefix(r.URL.Path, "/admin/codex") {
		return api.serveAdmin(w, r, api.codex)
	}
	if strings.HasPrefix(r.URL.Path, "/admin/antigravity") {
		if !api.authorizeAdmin(w, r) {
			return true
		}
		switch {
		case r.URL.Path == "/admin/antigravity/models/sync" && r.Method == http.MethodPost:
			api.antigravitySync(w, r)
		case r.URL.Path == "/admin/antigravity/models/verify" && r.Method == http.MethodPost:
			api.antigravityVerify(w, r)
		default:
			http.NotFound(w, r)
		}
		return true
	}
	for _, route := range api.providerHandlers {
		if strings.HasPrefix(r.URL.Path, route.prefix) {
			return api.serveAdmin(w, r, route.handler)
		}
	}
	return false
}

func (api *ProviderOperationsAPI) serveAdmin(w http.ResponseWriter, r *http.Request, handler http.HandlerFunc) bool {
	if !api.authorizeAdmin(w, r) {
		return true
	}
	handler(w, r)
	return true
}

func (h *proxyHandler) providerOperationsAPIService() *ProviderOperationsAPI {
	if h.providerOperationsAPI != nil {
		return h.providerOperationsAPI
	}
	return &ProviderOperationsAPI{
		authorizeAdmin:      h.checkAdminAuth,
		codex:               h.serveCodexAdmin,
		antigravityCallback: h.handleAntigravityCallback,
		antigravitySync:     h.handleAntigravityModelSync,
		antigravityVerify:   h.handleAntigravityModelVerify,
		providerHandlers: []providerOperationHandler{
			{prefix: "/admin/kimi-platform", handler: h.serveKimiPlatformAdmin},
			{prefix: "/admin/kimi", handler: h.serveKimiAdmin},
			{prefix: "/admin/minimax", handler: h.serveMinimaxAdmin},
			{prefix: "/admin/zai", handler: h.serveZAIAdmin},
			{prefix: "/admin/xiaomi", handler: h.serveXiaomiAdmin},
			{prefix: "/admin/grok", handler: h.serveGrokAdmin},
			{prefix: "/admin/deepseek", handler: h.serveDeepSeekAdmin},
			{prefix: "/admin/qwen", handler: h.serveQwenAdmin},
			{prefix: "/admin/openrouter", handler: h.serveOpenRouterAdmin},
			{prefix: "/admin/nvidia", handler: h.serveNvidiaAdmin},
			{prefix: "/admin/bfl", handler: h.serveBFLAdmin},
		},
	}
}
