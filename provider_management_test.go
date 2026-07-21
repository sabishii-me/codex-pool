package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProviderAdminRoutesRequireElevatedAdminSession(t *testing.T) {
	h, user, secret := newTestHandlerWithSession(t)
	h.pool = newPoolState([]*Account{{ID: "kimi", Type: AccountTypeKimi}}, false)

	// Signed in, but not admin-listed at all.
	notAdminRequest := httptest.NewRequest(http.MethodGet, "/admin/kimi", nil)
	notAdminRequest.AddCookie(newTestSessionCookie(t, secret, user))
	notAdminResponse := httptest.NewRecorder()
	h.ServeHTTP(notAdminResponse, notAdminRequest)
	if notAdminResponse.Code != http.StatusForbidden {
		t.Fatalf("non-admin session status = %d, want 403", notAdminResponse.Code)
	}

	// Admin-listed, MFA enrolled, but not yet elevated this session.
	h.cfg.adminEmails = []string{user.Email}
	store, err := newAdminTOTPStore(filepath.Join(t.TempDir(), "admin_mfa.json"))
	if err != nil {
		t.Fatalf("newAdminTOTPStore failed: %v", err)
	}
	h.adminTOTP = store
	totpSecret := generateTOTPSecret()
	if err := store.Save(&AdminTOTPSecret{Email: user.Email, Secret: totpSecret, Confirmed: true, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("failed to save admin TOTP entry: %v", err)
	}

	unelevatedRequest := httptest.NewRequest(http.MethodGet, "/admin/kimi", nil)
	unelevatedRequest.AddCookie(newTestSessionCookie(t, secret, user))
	unelevatedResponse := httptest.NewRecorder()
	h.ServeHTTP(unelevatedResponse, unelevatedRequest)
	if unelevatedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("admin-listed but unelevated status = %d, want 401", unelevatedResponse.Code)
	}

	// Elevate via a real TOTP verify call, then the same route should succeed.
	code, err := totpCodeAt(totpSecret, time.Now())
	if err != nil {
		t.Fatalf("totpCodeAt failed: %v", err)
	}
	verifyRequest := httptest.NewRequest(http.MethodPost, "/api/admin/mfa/verify", strings.NewReader(`{"code":"`+code+`"}`))
	verifyRequest.AddCookie(newTestSessionCookie(t, secret, user))
	verifyResponse := httptest.NewRecorder()
	h.ServeHTTP(verifyResponse, verifyRequest)
	if verifyResponse.Code != http.StatusOK {
		t.Fatalf("mfa verify status = %d, body=%s", verifyResponse.Code, verifyResponse.Body.String())
	}

	elevatedRequest := httptest.NewRequest(http.MethodGet, "/admin/kimi", nil)
	elevatedRequest.AddCookie(newTestSessionCookie(t, secret, user))
	for _, cookie := range verifyResponse.Result().Cookies() {
		elevatedRequest.AddCookie(cookie)
	}
	elevatedResponse := httptest.NewRecorder()
	h.ServeHTTP(elevatedResponse, elevatedRequest)
	if elevatedResponse.Code != http.StatusOK {
		t.Fatalf("elevated admin status = %d, want 200: %s", elevatedResponse.Code, elevatedResponse.Body.String())
	}
}

func TestSetAccountDisabledPersistsAndReloads(t *testing.T) {
	file := filepath.Join(t.TempDir(), "kimi.json")
	if err := os.WriteFile(file, []byte(`{"api_key":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	account := &Account{ID: "kimi", Type: AccountTypeKimi, File: file, AccessToken: "secret"}
	h := &proxyHandler{pool: newPoolState([]*Account{account}, false)}

	recorder := httptest.NewRecorder()
	h.setAccountDisabled(recorder, account.ID, true)
	if recorder.Code != http.StatusOK || !account.Disabled {
		t.Fatalf("disable response=%d account.disabled=%v", recorder.Code, account.Disabled)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if root["disabled"] != true {
		t.Fatalf("persisted disabled = %#v, want true", root["disabled"])
	}

	provider := NewKimiProvider(nil)
	reloaded, err := provider.LoadAccount(filepath.Base(file), file, data)
	if err != nil {
		t.Fatal(err)
	}
	applyCommonAccountFileState(reloaded, data)
	if !reloaded.Disabled {
		t.Fatal("reloaded account lost disabled state")
	}
}
