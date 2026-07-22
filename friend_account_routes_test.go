package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestHandlerWithSession builds a proxyHandler wired for the Google
// OAuth-gated session flow: a real GatewayUserStore, a JWT secret via
// POOL_JWT_SECRET, and one pre-created allowlisted user. The Google client id
// is set (non-empty) so checkAdminOrSessionAuth exercises real auth instead
// of the "nothing configured -> open deployment" fallback.
func newTestHandlerWithSession(t *testing.T) (h *proxyHandler, user *GatewayUser, secret string) {
	t.Helper()
	secret = "test-secret-key-12345678901234567890"
	t.Setenv("POOL_JWT_SECRET", secret)

	store, err := newGatewayUserStore(filepath.Join(t.TempDir(), "pool_users.json"))
	if err != nil {
		t.Fatalf("newGatewayUserStore failed: %v", err)
	}
	user = &GatewayUser{ID: "user123", Token: "dl-token", Email: "friend@example.com", PlanType: "pro", CreatedAt: time.Now()}
	if err := store.Create(user); err != nil {
		t.Fatalf("failed to create pool user: %v", err)
	}

	h = &proxyHandler{
		cfg:       &config{oauthGoogleClientID: "test-client-id", allowedEmails: []string{"friend@example.com"}},
		poolUsers: store,
	}
	return h, user, secret
}

func newTestSessionCookie(t *testing.T, secret string, user *GatewayUser) *http.Cookie {
	t.Helper()
	token, err := signJWT(secret, map[string]any{
		"typ":   "session",
		"sub":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("signJWT failed: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: token}
}

func TestSessionAuthRejectsMissingOrInvalidCookie(t *testing.T) {
	h, _, _ := newTestHandlerWithSession(t)

	noCookieRequest := httptest.NewRequest(http.MethodGet, "/api/pool/stats", nil)
	noCookieResponse := httptest.NewRecorder()
	if h.checkAdminOrSessionAuth(noCookieResponse, noCookieRequest) {
		t.Fatal("missing session cookie must not authenticate")
	}
	if noCookieResponse.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", noCookieResponse.Code, http.StatusUnauthorized)
	}

	badCookieRequest := httptest.NewRequest(http.MethodGet, "/api/pool/stats", nil)
	badCookieRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "garbage"})
	badCookieResponse := httptest.NewRecorder()
	if h.checkAdminOrSessionAuth(badCookieResponse, badCookieRequest) {
		t.Fatal("invalid session cookie must not authenticate")
	}
}

func TestSessionAuthAcceptsValidCookie(t *testing.T) {
	h, user, secret := newTestHandlerWithSession(t)

	request := httptest.NewRequest(http.MethodGet, "/api/pool/stats", nil)
	request.AddCookie(newTestSessionCookie(t, secret, user))
	response := httptest.NewRecorder()
	if !h.checkAdminOrSessionAuth(response, request) {
		t.Fatalf("valid session cookie rejected with status %d", response.Code)
	}
}

func TestSignedInFriendCanStartCodexAccountContributionWithoutAdminAccess(t *testing.T) {
	h, user, secret := newTestHandlerWithSession(t)

	request := httptest.NewRequest(http.MethodPost, "/api/pool/accounts/codex/add", strings.NewReader(`{"redirect_port":1455}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(newTestSessionCookie(t, secret, user))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"oauth_url"`) || !strings.Contains(body, `"verifier"`) {
		t.Fatalf("missing OAuth contribution payload: %s", body)
	}
}

func TestFriendAccountContributionRequiresAuthentication(t *testing.T) {
	h, _, _ := newTestHandlerWithSession(t)
	request := httptest.NewRequest(http.MethodPost, "/api/pool/accounts/codex/add", nil)
	response := httptest.NewRecorder()

	h.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}
