package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Admin two-factor authentication: TOTP (RFC 6238) as the primary factor,
// with one-time recovery codes as the standard fallback if the
// authenticator device is lost - the same pattern GitHub/Google use.
// Implemented from stdlib crypto only; go.mod has no TOTP/QR dependency and
// this keeps it that way.

const (
	totpDigits  = 6
	totpModulus = 1_000_000
	totpStep    = 30 * time.Second
	// Real phone clocks commonly drift by more than the RFC's implied
	// single step - confirmed live (a phone running ~2 minutes fast
	// produced a code that only validated once this was widened to 4).
	totpDrift = 4 // allowed steps of clock drift, each direction (4 * 30s = 2 minutes)

	adminElevatedCookieName = "admin_elevated"
	adminElevatedMaxAge     = 8 * time.Hour

	recoveryCodeCount = 10
)

var totpBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func generateTOTPSecret() string {
	b := make([]byte, 20)
	rand.Read(b)
	return totpBase32.EncodeToString(b)
}

func totpCodeAt(secret string, t time.Time) (string, error) {
	key, err := totpBase32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	counter := uint64(t.Unix() / int64(totpStep.Seconds()))
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	h := hmac.New(sha1.New, key)
	h.Write(buf)
	sum := h.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	code := truncated % totpModulus
	return fmt.Sprintf("%0*d", totpDigits, code), nil
}

// verifyTOTP checks code against the current time step and totpDrift steps
// on either side, to tolerate real-world clock drift between server and
// phone (see totpDrift's comment).
func verifyTOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	now := time.Now()
	for drift := -totpDrift; drift <= totpDrift; drift++ {
		want, err := totpCodeAt(secret, now.Add(time.Duration(drift)*totpStep))
		if err != nil {
			return false
		}
		if hmac.Equal([]byte(want), []byte(code)) {
			return true
		}
	}
	return false
}

func totpOTPAuthURL(issuer, email, secret string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	label := url.PathEscape(issuer) + ":" + url.PathEscape(email)
	return fmt.Sprintf("otpauth://totp/%s?%s", label, v.Encode())
}

// RecoveryCode stores only a hash - the plaintext code is shown to the
// admin exactly once, at generation time, and never persisted.
type RecoveryCode struct {
	Hash   string     `json:"hash"`
	UsedAt *time.Time `json:"used_at,omitempty"`
}

func hashRecoveryCode(code string) string {
	normalized := strings.ToLower(strings.TrimSpace(code))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func randomRecoveryCode() string {
	raw := randomHex(6) // 12 hex chars
	return raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12]
}

func generateRecoveryCodes(n int) (plaintext []string, codes []RecoveryCode) {
	plaintext = make([]string, n)
	codes = make([]RecoveryCode, n)
	for i := 0; i < n; i++ {
		code := randomRecoveryCode()
		plaintext[i] = code
		codes[i] = RecoveryCode{Hash: hashRecoveryCode(code)}
	}
	return plaintext, codes
}

// consumeRecoveryCode checks code against entry's unused codes, marking the
// matching one used on success. The caller must persist entry afterward.
func consumeRecoveryCode(entry *AdminTOTPSecret, code string) bool {
	hash := hashRecoveryCode(code)
	now := time.Now()
	for i := range entry.RecoveryCodes {
		rc := &entry.RecoveryCodes[i]
		if rc.UsedAt != nil {
			continue
		}
		if hmac.Equal([]byte(rc.Hash), []byte(hash)) {
			rc.UsedAt = &now
			return true
		}
	}
	return false
}

func countUnusedRecoveryCodes(entry *AdminTOTPSecret) int {
	count := 0
	for _, rc := range entry.RecoveryCodes {
		if rc.UsedAt == nil {
			count++
		}
	}
	return count
}

// AdminTOTPSecret is one admin's enrolled second factor.
type AdminTOTPSecret struct {
	Email         string         `json:"email"`
	Secret        string         `json:"secret"`
	Confirmed     bool           `json:"confirmed"`
	RecoveryCodes []RecoveryCode `json:"recovery_codes"`
	CreatedAt     time.Time      `json:"created_at"`
}

// AdminTOTPStore persists admin MFA secrets to a JSON file, mirroring
// GatewayUserStore's load/save pattern in pool_users.go.
type AdminTOTPStore struct {
	mu      sync.RWMutex
	path    string
	secrets map[string]*AdminTOTPSecret // keyed by lowercased email
}

func newAdminTOTPStore(path string) (*AdminTOTPStore, error) {
	s := &AdminTOTPStore{path: path, secrets: make(map[string]*AdminTOTPSecret)}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *AdminTOTPStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var list []*AdminTOTPSecret
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets = make(map[string]*AdminTOTPSecret, len(list))
	for _, entry := range list {
		s.secrets[entry.Email] = entry
	}
	return nil
}

func (s *AdminTOTPStore) save() error {
	list := make([]*AdminTOTPSecret, 0, len(s.secrets))
	for _, entry := range s.secrets {
		list = append(list, entry)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func (s *AdminTOTPStore) Get(email string) *AdminTOTPSecret {
	email = strings.ToLower(strings.TrimSpace(email))
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.secrets[email]
}

func (s *AdminTOTPStore) Save(entry *AdminTOTPSecret) error {
	entry.Email = strings.ToLower(strings.TrimSpace(entry.Email))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[entry.Email] = entry
	return s.save()
}

// getAdminMFAPath returns the admin MFA store path from env or a default,
// mirroring getPoolUsersPath()'s precedence convention.
func getAdminMFAPath() string {
	if v := os.Getenv("ADMIN_MFA_PATH"); v != "" {
		return v
	}
	return "./data/admin_mfa.json"
}

// adminEmailAllowed reports whether email exactly matches an entry in list.
// Deliberately not domain-wildcard like emailAllowed() in oauth_login.go -
// granting operator power to an entire domain is far higher blast radius
// than just letting the domain sign in.
func adminEmailAllowed(list []string, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, entry := range list {
		if entry == email {
			return true
		}
	}
	return false
}

func setAdminElevatedCookie(w http.ResponseWriter, r *http.Request, userID string) error {
	token, err := signJWT(getPoolJWTSecret(), map[string]any{
		"typ": "admin",
		"sub": userID,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(adminElevatedMaxAge).Unix(),
	})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminElevatedCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(adminElevatedMaxAge.Seconds()),
	})
	return nil
}

func clearAdminElevatedCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: adminElevatedCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: cookieIsSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// adminElevated reports whether the request carries a valid, unexpired
// admin_elevated cookie issued to userID.
func adminElevated(r *http.Request, userID string) bool {
	cookie, err := r.Cookie(adminElevatedCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	claims, err := validatePoolUserJWT(getPoolJWTSecret(), cookie.Value)
	if err != nil {
		return false
	}
	if typ, _ := claims["typ"].(string); typ != "admin" {
		return false
	}
	sub, _ := claims["sub"].(string)
	return sub == userID
}

// requireAdminIdentity resolves the signed-in session and confirms the
// email is admin-listed. Used by the MFA endpoints themselves, which can't
// go through checkAdminAuth (that requires elevation, which doesn't exist
// yet during enrollment/verification).
func (h *proxyHandler) requireAdminIdentity(w http.ResponseWriter, r *http.Request) (*GatewayUser, bool) {
	user, ok := h.sessionUser(r)
	if !ok {
		respondJSONError(w, http.StatusUnauthorized, "not signed in")
		return nil, false
	}
	if !adminEmailAllowed(h.cfg.adminEmails, user.Email) {
		respondJSONError(w, http.StatusForbidden, "not an admin")
		return nil, false
	}
	return user, true
}

type mfaStatusResponse struct {
	Enrolled               bool `json:"enrolled"`
	Elevated               bool `json:"elevated"`
	RecoveryCodesRemaining int  `json:"recovery_codes_remaining"`
}

// GET /api/admin/mfa/status
func (h *proxyHandler) handleMFAStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := h.requireAdminIdentity(w, r)
	if !ok {
		return
	}
	resp := mfaStatusResponse{}
	if h.adminTOTP != nil {
		if entry := h.adminTOTP.Get(user.Email); entry != nil && entry.Confirmed {
			resp.Enrolled = true
			resp.RecoveryCodesRemaining = countUnusedRecoveryCodes(entry)
		}
	}
	resp.Elevated = adminElevated(r, user.ID)
	respondJSON(w, resp)
}

// POST /api/admin/mfa/enroll
func (h *proxyHandler) handleMFAEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := h.requireAdminIdentity(w, r)
	if !ok {
		return
	}
	if h.adminTOTP == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "MFA store not configured")
		return
	}
	existing := h.adminTOTP.Get(user.Email)
	if existing != nil && existing.Confirmed {
		respondJSONError(w, http.StatusConflict, "MFA already enrolled - use regenerate instead")
		return
	}

	// Reuse a still-pending secret instead of rotating it on every call.
	// Without this, reopening the enrollment dialog (page reload, retry,
	// switching tabs and back) silently invalidates a QR code the admin
	// may have already scanned into their authenticator app.
	secret := ""
	if existing != nil {
		secret = existing.Secret
	} else {
		secret = generateTOTPSecret()
		entry := &AdminTOTPSecret{Email: user.Email, Secret: secret, CreatedAt: time.Now()}
		if err := h.adminTOTP.Save(entry); err != nil {
			respondJSONError(w, http.StatusInternalServerError, "failed to save MFA secret")
			return
		}
	}
	respondJSON(w, map[string]string{
		"secret":      secret,
		"otpauth_url": totpOTPAuthURL("codex-pool", user.Email, secret),
	})
}

// POST /api/admin/mfa/confirm {code}
// debugLogTOTPMismatch logs enough context to diagnose a failed TOTP check
// without ever logging the secret itself: whether the submitted code
// matches any step in a 6-minute window around now (revealing clock drift
// beyond the normal ±1 step tolerance, or confirming it's not a drift issue
// at all because nothing in a wide window matches).
func debugLogTOTPMismatch(email, secret, submitted string) {
	trimmed := strings.TrimSpace(submitted)
	now := time.Now()
	var lines []string
	matched := false
	for i := -6; i <= 6; i++ {
		t := now.Add(time.Duration(i) * totpStep)
		code, err := totpCodeAt(secret, t)
		if err != nil {
			continue
		}
		marker := ""
		if code == trimmed {
			marker = " <== MATCHES SUBMITTED CODE"
			matched = true
		}
		lines = append(lines, fmt.Sprintf("  step %+d (%s): %s%s", i, t.Format("15:04:05"), code, marker))
	}
	log.Printf("mfa verify mismatch for %s: submitted=%q (len=%d) server_time=%s matched_in_window=%v\n%s",
		email, trimmed, len(trimmed), now.Format(time.RFC3339), matched, strings.Join(lines, "\n"))
}

func (h *proxyHandler) handleMFAConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := h.requireAdminIdentity(w, r)
	if !ok {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if h.adminTOTP == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "MFA store not configured")
		return
	}
	entry := h.adminTOTP.Get(user.Email)
	if entry == nil {
		respondJSONError(w, http.StatusBadRequest, "no pending MFA enrollment - call enroll first")
		return
	}
	if entry.Confirmed {
		respondJSONError(w, http.StatusConflict, "MFA already confirmed")
		return
	}
	if !verifyTOTP(entry.Secret, req.Code) {
		debugLogTOTPMismatch(user.Email, entry.Secret, req.Code)
		respondJSONError(w, http.StatusUnauthorized, "invalid code")
		return
	}
	plaintext, codes := generateRecoveryCodes(recoveryCodeCount)
	entry.Confirmed = true
	entry.RecoveryCodes = codes
	if err := h.adminTOTP.Save(entry); err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to save MFA secret")
		return
	}
	if err := setAdminElevatedCookie(w, r, user.ID); err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to elevate session")
		return
	}
	respondJSON(w, map[string]any{"success": true, "recovery_codes": plaintext})
}

// POST /api/admin/mfa/verify {code} or {recovery_code}
func (h *proxyHandler) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := h.requireAdminIdentity(w, r)
	if !ok {
		return
	}
	var req struct {
		Code         string `json:"code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if h.adminTOTP == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "MFA store not configured")
		return
	}
	entry := h.adminTOTP.Get(user.Email)
	if entry == nil || !entry.Confirmed {
		respondJSONError(w, http.StatusBadRequest, "MFA not enrolled")
		return
	}

	ip := getClientIP(r)
	if h.bruteForce != nil && h.bruteForce.isBanned(ip) {
		http.Error(w, "too many failed attempts, try again later", http.StatusTooManyRequests)
		return
	}

	valid := false
	usedRecovery := false
	if strings.TrimSpace(req.Code) != "" {
		valid = verifyTOTP(entry.Secret, req.Code)
	} else if strings.TrimSpace(req.RecoveryCode) != "" {
		valid = consumeRecoveryCode(entry, req.RecoveryCode)
		usedRecovery = valid
	}
	if !valid {
		if strings.TrimSpace(req.Code) != "" {
			debugLogTOTPMismatch(user.Email, entry.Secret, req.Code)
		}
		if h.bruteForce != nil {
			h.bruteForce.recordFailure(ip)
		}
		respondJSONError(w, http.StatusUnauthorized, "invalid code")
		return
	}
	if h.bruteForce != nil {
		h.bruteForce.recordSuccess(ip)
	}
	if usedRecovery {
		if err := h.adminTOTP.Save(entry); err != nil {
			respondJSONError(w, http.StatusInternalServerError, "failed to persist recovery code use")
			return
		}
	}
	if err := setAdminElevatedCookie(w, r, user.ID); err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to elevate session")
		return
	}
	respondJSON(w, map[string]any{"success": true})
}

// POST /api/admin/mfa/regenerate - requires an already-elevated session;
// issues a brand new secret + recovery codes, invalidating the old ones.
func (h *proxyHandler) handleMFARegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := h.requireAdminIdentity(w, r)
	if !ok {
		return
	}
	if !adminElevated(r, user.ID) {
		respondJSONError(w, http.StatusUnauthorized, "two-factor verification required")
		return
	}
	if h.adminTOTP == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "MFA store not configured")
		return
	}
	secret := generateTOTPSecret()
	plaintext, codes := generateRecoveryCodes(recoveryCodeCount)
	entry := &AdminTOTPSecret{Email: user.Email, Secret: secret, Confirmed: true, RecoveryCodes: codes, CreatedAt: time.Now()}
	if err := h.adminTOTP.Save(entry); err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to save MFA secret")
		return
	}
	respondJSON(w, map[string]any{
		"secret":         secret,
		"otpauth_url":    totpOTPAuthURL("codex-pool", user.Email, secret),
		"recovery_codes": plaintext,
	})
}

// POST /api/admin/mfa/regenerate-codes - requires elevation; keeps the
// existing TOTP secret, issues a fresh set of recovery codes.
func (h *proxyHandler) handleMFARegenerateCodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := h.requireAdminIdentity(w, r)
	if !ok {
		return
	}
	if !adminElevated(r, user.ID) {
		respondJSONError(w, http.StatusUnauthorized, "two-factor verification required")
		return
	}
	if h.adminTOTP == nil {
		respondJSONError(w, http.StatusServiceUnavailable, "MFA store not configured")
		return
	}
	entry := h.adminTOTP.Get(user.Email)
	if entry == nil || !entry.Confirmed {
		respondJSONError(w, http.StatusBadRequest, "MFA not enrolled")
		return
	}
	plaintext, codes := generateRecoveryCodes(recoveryCodeCount)
	entry.RecoveryCodes = codes
	if err := h.adminTOTP.Save(entry); err != nil {
		respondJSONError(w, http.StatusInternalServerError, "failed to save recovery codes")
		return
	}
	respondJSON(w, map[string]any{"recovery_codes": plaintext})
}
