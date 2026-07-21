package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Google OAuth login gate: replaces the old shared friend_code with a real
// "Sign in with Google" flow, restricted to an admin-maintained email
// allowlist. This is a standard server-side (confidential-client) OAuth
// Authorization Code flow with a full top-level redirect - unlike the
// Antigravity account-contribution flow in admin_antigravity.go, there's no
// popup and no PKCE, since that flow impersonates a public native-app client
// from inside an already-open SPA tab, while this one is our own registered
// web client authenticating the browser before the SPA loads at all.

const (
	googleOAuthAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	googleOAuthTokenURL     = "https://oauth2.googleapis.com/token"
	googleUserInfoURL       = "https://openidconnect.googleapis.com/v1/userinfo"

	oauthStateCookieName = "oauth_state"
	sessionCookieName    = "pool_session"
	sessionMaxAge        = 30 * 24 * time.Hour
)

func googleOAuthRedirectURI(h *proxyHandler, r *http.Request) string {
	if v := strings.TrimSpace(os.Getenv("OAUTH_GOOGLE_REDIRECT_URI")); v != "" {
		return v
	}
	return strings.TrimRight(h.getEffectivePublicURL(r), "/") + "/auth/callback/google"
}

func cookieIsSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// emailAllowed reports whether email is on the configured allowlist. An
// empty allowlist denies everyone - enabling the Google client id without
// setting allowed_emails should fail closed, not open the pool to any
// Google account. Entries without an "@" are treated as a bare domain
// (e.g. "example.com") and match any address on that domain; entries with
// an "@" must match the full address exactly.
func emailAllowed(allowlist []string, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	domain := ""
	if at := strings.LastIndex(email, "@"); at >= 0 {
		domain = email[at+1:]
	}
	for _, entry := range allowlist {
		if entry == email {
			return true
		}
		if domain != "" && !strings.Contains(entry, "@") && entry == domain {
			return true
		}
	}
	return false
}

// handleGoogleLoginStart begins the login flow: GET /auth/login/google.
func (h *proxyHandler) handleGoogleLoginStart(w http.ResponseWriter, r *http.Request) {
	if h.cfg.oauthGoogleClientID == "" {
		http.Error(w, "Google sign-in is not configured", http.StatusServiceUnavailable)
		return
	}

	state := randomHex(24)
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})

	u, _ := url.Parse(googleOAuthAuthorizeURL)
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", h.cfg.oauthGoogleClientID)
	q.Set("redirect_uri", googleOAuthRedirectURI(h, r))
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("prompt", "select_account")
	u.RawQuery = q.Encode()

	http.Redirect(w, r, u.String(), http.StatusFound)
}

// handleGoogleLoginCallback completes the flow: GET /auth/callback/google.
// Google redirects here as a top-level navigation, so failures redirect back
// to "/" with an ?error= query param rather than returning a JSON error -
// there's no SPA listening yet at this point.
func (h *proxyHandler) handleGoogleLoginCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if upstreamErr := strings.TrimSpace(r.URL.Query().Get("error")); upstreamErr != "" {
		http.Redirect(w, r, "/?error=oauth_failed", http.StatusFound)
		return
	}

	state := strings.TrimSpace(r.URL.Query().Get("state"))
	stateCookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || state == "" || stateCookie.Value != state {
		http.Redirect(w, r, "/?error=oauth_failed", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: cookieIsSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		http.Redirect(w, r, "/?error=oauth_failed", http.StatusFound)
		return
	}

	email, verified, err := h.exchangeGoogleCode(r, code)
	if err != nil || !verified || email == "" {
		log.Printf("google oauth: exchange failed: %v", err)
		http.Redirect(w, r, "/?error=oauth_failed", http.StatusFound)
		return
	}
	email = strings.ToLower(strings.TrimSpace(email))

	if !emailAllowed(h.cfg.allowedEmails, email) {
		http.Redirect(w, r, "/?error=not_allowed", http.StatusFound)
		return
	}

	if h.poolUsers == nil {
		http.Redirect(w, r, "/?error=system_error", http.StatusFound)
		return
	}

	user := h.poolUsers.GetByEmail(email)
	if user == nil {
		user = &PoolUser{
			ID:        randomHex(8),
			Token:     randomHex(16),
			Email:     email,
			PlanType:  "pro",
			CreatedAt: time.Now(),
		}
		if err := h.poolUsers.Create(user); err != nil {
			log.Printf("failed to create pool user for %s: %v", email, err)
			http.Redirect(w, r, "/?error=system_error", http.StatusFound)
			return
		}
	} else if user.Disabled {
		http.Redirect(w, r, "/?error=not_allowed", http.StatusFound)
		return
	}

	token, err := signJWT(getPoolJWTSecret(), map[string]any{
		"typ":   "session",
		"sub":   user.ID,
		"email": user.Email,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(sessionMaxAge).Unix(),
	})
	if err != nil {
		http.Redirect(w, r, "/?error=system_error", http.StatusFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionMaxAge.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleLogout clears the session cookie: POST /auth/logout.
func (h *proxyHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: cookieIsSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	clearAdminElevatedCookie(w, r)
	w.WriteHeader(http.StatusOK)
}

// handlePoolSession returns the CLI-credential bundle for the current
// session: GET /api/pool/session. This replaces the old POST
// /api/friend/claim - the SPA calls it on boot to hydrate its session
// instead of re-submitting a shared secret.
func (h *proxyHandler) handlePoolSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := h.sessionUser(r)
	if !ok {
		respondJSONError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	h.writeFriendSessionJSON(w, r, user)
}

// sessionUser resolves the pool_session cookie to a live, enabled PoolUser.
// Used both by handlePoolSession and by checkAdminOrSessionAuth in router.go.
func (h *proxyHandler) sessionUser(r *http.Request) (*PoolUser, bool) {
	if h.poolUsers == nil {
		return nil, false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	claims, err := validatePoolUserJWT(getPoolJWTSecret(), cookie.Value)
	if err != nil {
		return nil, false
	}
	if typ, _ := claims["typ"].(string); typ != "session" {
		return nil, false
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, false
	}
	user := h.poolUsers.Get(sub)
	if user == nil || user.Disabled {
		return nil, false
	}
	return user, true
}

// exchangeGoogleCode exchanges an authorization code for an access token,
// then calls Google's OpenID Connect userinfo endpoint to get the verified
// email. This is a standard server-to-server OAuth call over TLS using our
// own client_secret - no need to verify Google's signed ID token ourselves
// (which would require fetching and caching their JWKS).
func (h *proxyHandler) exchangeGoogleCode(r *http.Request, code string) (email string, verified bool, err error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", h.cfg.oauthGoogleClientID)
	form.Set("client_secret", h.cfg.oauthGoogleClientSecret)
	form.Set("redirect_uri", googleOAuthRedirectURI(h, r))

	tokenReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, googleOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", false, err
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		return "", false, err
	}
	defer tokenResp.Body.Close()
	tokenBody, _ := io.ReadAll(tokenResp.Body)
	if tokenResp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("token exchange status=%s body=%s", tokenResp.Status, string(tokenBody))
	}

	var tokenPayload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(tokenBody, &tokenPayload); err != nil || tokenPayload.AccessToken == "" {
		return "", false, fmt.Errorf("token exchange: missing access_token")
	}

	userReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return "", false, err
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenPayload.AccessToken)

	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		return "", false, err
	}
	defer userResp.Body.Close()
	userBody, _ := io.ReadAll(userResp.Body)
	if userResp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("userinfo status=%s body=%s", userResp.Status, string(userBody))
	}

	var info struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := json.Unmarshal(userBody, &info); err != nil {
		return "", false, err
	}
	return info.Email, info.EmailVerified, nil
}
