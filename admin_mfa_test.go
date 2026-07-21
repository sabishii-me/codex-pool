package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTOTPGenerateAndVerifyRoundTrip(t *testing.T) {
	secret := generateTOTPSecret()
	if secret == "" {
		t.Fatal("generateTOTPSecret returned empty string")
	}

	code, err := totpCodeAt(secret, time.Now())
	if err != nil {
		t.Fatalf("totpCodeAt failed: %v", err)
	}
	if len(code) != totpDigits {
		t.Fatalf("code length = %d, want %d", len(code), totpDigits)
	}
	if !verifyTOTP(secret, code) {
		t.Fatal("verifyTOTP rejected a just-generated code")
	}
}

func TestTOTPVerifyRejectsWrongCode(t *testing.T) {
	secret := generateTOTPSecret()
	if verifyTOTP(secret, "000000") {
		t.Fatal("verifyTOTP accepted an arbitrary wrong code (astronomically unlikely unless broken)")
	}
	if verifyTOTP(secret, "") {
		t.Fatal("verifyTOTP accepted an empty code")
	}
}

func TestTOTPVerifyToleratesClockDriftWithinWindow(t *testing.T) {
	secret := generateTOTPSecret()
	past := time.Now().Add(-totpStep) // one step behind
	code, err := totpCodeAt(secret, past)
	if err != nil {
		t.Fatalf("totpCodeAt failed: %v", err)
	}
	if !verifyTOTP(secret, code) {
		t.Fatal("verifyTOTP rejected a code one step of drift away")
	}
}

// Regression test for a real incident: a phone with its clock running
// ~2 minutes fast produced a code that was rejected until totpDrift was
// widened from 1 to 4 steps (30s each, so 4 steps = 2 minutes). Locks in
// that this class of ordinary real-world clock drift is now tolerated.
func TestTOTPVerifyToleratesTwoMinutesOfDrift(t *testing.T) {
	secret := generateTOTPSecret()
	future := time.Now().Add(4 * totpStep) // phone clock 2 minutes fast
	code, err := totpCodeAt(secret, future)
	if err != nil {
		t.Fatalf("totpCodeAt failed: %v", err)
	}
	if !verifyTOTP(secret, code) {
		t.Fatal("verifyTOTP rejected a code from a clock running 2 minutes fast")
	}
}

func TestTOTPVerifyRejectsOutOfWindowCode(t *testing.T) {
	secret := generateTOTPSecret()
	farPast := time.Now().Add(-10 * totpStep)
	code, err := totpCodeAt(secret, farPast)
	if err != nil {
		t.Fatalf("totpCodeAt failed: %v", err)
	}
	if verifyTOTP(secret, code) {
		t.Fatal("verifyTOTP accepted a code far outside the drift window")
	}
}

func TestRecoveryCodeConsumedOnce(t *testing.T) {
	plaintext, codes := generateRecoveryCodes(10)
	if len(plaintext) != 10 || len(codes) != 10 {
		t.Fatalf("expected 10 codes, got %d plaintext / %d hashes", len(plaintext), len(codes))
	}
	entry := &AdminTOTPSecret{Email: "admin@example.com", RecoveryCodes: codes}

	target := plaintext[3]
	if !consumeRecoveryCode(entry, target) {
		t.Fatal("consumeRecoveryCode rejected a valid, unused code")
	}
	if countUnusedRecoveryCodes(entry) != 9 {
		t.Fatalf("unused count = %d, want 9", countUnusedRecoveryCodes(entry))
	}
	if consumeRecoveryCode(entry, target) {
		t.Fatal("consumeRecoveryCode accepted an already-used code")
	}
}

func TestRecoveryCodeRejectsUnknownCode(t *testing.T) {
	_, codes := generateRecoveryCodes(10)
	entry := &AdminTOTPSecret{Email: "admin@example.com", RecoveryCodes: codes}
	if consumeRecoveryCode(entry, "0000-0000-0000") {
		t.Fatal("consumeRecoveryCode accepted a code that was never issued")
	}
}

func TestRecoveryCodeHashesNeverStorePlaintext(t *testing.T) {
	plaintext, codes := generateRecoveryCodes(10)
	for i, code := range codes {
		if code.Hash == plaintext[i] {
			t.Fatal("recovery code hash equals its own plaintext")
		}
		for _, p := range plaintext {
			if code.Hash == p {
				t.Fatal("recovery code hash matches some plaintext code verbatim")
			}
		}
	}
}

func TestAdminEmailAllowedExactMatchOnly(t *testing.T) {
	list := []string{"admin@c0dt.app", "other@example.com"}

	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"exact match", "admin@c0dt.app", true},
		{"case insensitive", "Admin@C0dt.App", true},
		{"whitespace trimmed", "  admin@c0dt.app  ", true},
		{"not listed", "stranger@c0dt.app", false},
		{"empty email", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adminEmailAllowed(list, tt.email); got != tt.want {
				t.Errorf("adminEmailAllowed(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestAdminEmailAllowedDoesNotAcceptBareDomains(t *testing.T) {
	// Unlike emailAllowed() (the login gate, which supports bare-domain
	// entries), adminEmailAllowed must never grant operator power to an
	// entire domain - it was a deliberate, explicit choice to keep this
	// stricter than the login allowlist.
	list := []string{"c0dt.app"}
	if adminEmailAllowed(list, "anyone@c0dt.app") {
		t.Fatal("adminEmailAllowed must not treat a bare domain entry as a wildcard")
	}
}

func TestAdminTOTPStorePersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin_mfa.json")
	store, err := newAdminTOTPStore(path)
	if err != nil {
		t.Fatalf("newAdminTOTPStore failed: %v", err)
	}
	_, codes := generateRecoveryCodes(10)
	entry := &AdminTOTPSecret{Email: "Admin@Example.com", Secret: generateTOTPSecret(), Confirmed: true, RecoveryCodes: codes, CreatedAt: time.Now()}
	if err := store.Save(entry); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reloaded, err := newAdminTOTPStore(path)
	if err != nil {
		t.Fatalf("reload newAdminTOTPStore failed: %v", err)
	}
	// Email is normalized to lowercase for lookup.
	got := reloaded.Get("admin@example.com")
	if got == nil {
		t.Fatal("reloaded store lost the saved entry")
	}
	if !got.Confirmed || got.Secret != entry.Secret || len(got.RecoveryCodes) != 10 {
		t.Fatalf("reloaded entry mismatch: %+v", got)
	}
}

// Regression test: reopening the "set up 2FA" dialog (page reload, retry,
// switching tabs and back) must not rotate a still-pending secret - doing
// so would silently invalidate a QR code the admin already scanned into
// their authenticator app, producing "invalid code" for a correctly-typed,
// correctly-computed code.
func TestMFAEnrollIsIdempotentWhilePending(t *testing.T) {
	h, user, sessionSecret := newTestHandlerWithSession(t)
	h.cfg.adminEmails = []string{user.Email}
	store, err := newAdminTOTPStore(filepath.Join(t.TempDir(), "admin_mfa.json"))
	if err != nil {
		t.Fatalf("newAdminTOTPStore failed: %v", err)
	}
	h.adminTOTP = store

	callEnroll := func() string {
		request := httptest.NewRequest(http.MethodPost, "/api/admin/mfa/enroll", nil)
		request.AddCookie(newTestSessionCookie(t, sessionSecret, user))
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("enroll status = %d, body=%s", response.Code, response.Body.String())
		}
		var result struct {
			Secret string `json:"secret"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to decode enroll response: %v", err)
		}
		return result.Secret
	}

	first := callEnroll()
	second := callEnroll()
	if first == "" {
		t.Fatal("enroll returned an empty secret")
	}
	if first != second {
		t.Fatalf("reopening enrollment rotated the secret: first=%q second=%q", first, second)
	}

	// The code for the reused secret must still confirm successfully.
	code, err := totpCodeAt(first, time.Now())
	if err != nil {
		t.Fatalf("totpCodeAt failed: %v", err)
	}
	confirmRequest := httptest.NewRequest(http.MethodPost, "/api/admin/mfa/confirm", strings.NewReader(`{"code":"`+code+`"}`))
	confirmRequest.AddCookie(newTestSessionCookie(t, sessionSecret, user))
	confirmResponse := httptest.NewRecorder()
	h.ServeHTTP(confirmResponse, confirmRequest)
	if confirmResponse.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, body=%s", confirmResponse.Code, confirmResponse.Body.String())
	}
}
