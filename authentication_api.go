package main

import "net/http"

// AuthenticationAPI owns login, callback, session, logout, and administrator
// MFA route dispatch. The handlers retain protocol-specific state, cookie, and
// verification behavior.
type AuthenticationAPI struct {
	googleLogin    http.HandlerFunc
	googleCallback http.HandlerFunc
	codexCallback  http.HandlerFunc
	logout         http.HandlerFunc
	session        http.HandlerFunc
	mfaHandlers    map[string]http.HandlerFunc
}

func (api *AuthenticationAPI) TryServe(w http.ResponseWriter, r *http.Request) bool {
	if api == nil || r == nil {
		return false
	}
	switch r.URL.Path {
	case "/auth/login/google":
		api.googleLogin(w, r)
		return true
	case "/auth/callback/google":
		api.googleCallback(w, r)
		return true
	case "/auth/callback/codex":
		if !requireMethod(w, r, http.MethodGet) {
			return true
		}
		api.codexCallback(w, r)
		return true
	case "/auth/logout":
		api.logout(w, r)
		return true
	case "/api/pool/session":
		api.session(w, r)
		return true
	}
	if handler := api.mfaHandlers[r.URL.Path]; handler != nil {
		handler(w, r)
		return true
	}
	return false
}

func (h *proxyHandler) authenticationAPIService() *AuthenticationAPI {
	if h.authenticationAPI != nil {
		return h.authenticationAPI
	}
	return &AuthenticationAPI{
		googleLogin:    h.handleGoogleLoginStart,
		googleCallback: h.handleGoogleLoginCallback,
		codexCallback:  h.handleCodexCallback,
		logout:         h.handleLogout,
		session:        h.handlePoolSession,
		mfaHandlers: map[string]http.HandlerFunc{
			"/api/admin/mfa/status":           h.handleMFAStatus,
			"/api/admin/mfa/enroll":           h.handleMFAEnroll,
			"/api/admin/mfa/confirm":          h.handleMFAConfirm,
			"/api/admin/mfa/verify":           h.handleMFAVerify,
			"/api/admin/mfa/regenerate":       h.handleMFARegenerate,
			"/api/admin/mfa/regenerate-codes": h.handleMFARegenerateCodes,
		},
	}
}
