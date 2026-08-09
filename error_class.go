package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

// ErrorClass categorises an upstream HTTP response or error so the retry loop
// can decide what to do: retry on another account, back off, give up, etc.
type ErrorClass int

const (
	// ErrorClassNone means no error — the request succeeded.
	ErrorClassNone ErrorClass = iota

	// ErrorClassTransient covers temporary server-side failures (408, 500-504).
	// Action: retry immediately on the next account.
	ErrorClassTransient

	// ErrorClassRateLimit means 429 Too Many Requests.
	// Action: exponential backoff on the account, then try another.
	ErrorClassRateLimit

	// ErrorClassAuth covers 401/403 — token expired or revoked.
	// Action: refresh token, retry same account once; if still failing, rotate.
	ErrorClassAuth

	// ErrorClassPayment means 402 — subscription lapsed, workspace deactivated.
	// Action: mark account dead, rotate.
	ErrorClassPayment

	// ErrorClassNotFound means 404 — model not available on this account.
	// Action: rotate to another account (don't penalise heavily).
	ErrorClassNotFound

	// ErrorClassInvalid means 400 — the request itself is bad.
	// Action: return error to client, do NOT retry.
	ErrorClassInvalid

	// ErrorClassFatal is a catch-all for non-retryable errors we don't have
	// a specific category for. Action: return error to client.
	ErrorClassFatal
)

// classifyStatus maps an HTTP status code to an ErrorClass.
func classifyStatus(statusCode int) ErrorClass {
	switch {
	case statusCode >= 200 && statusCode < 400:
		return ErrorClassNone

	case statusCode == http.StatusBadRequest: // 400
		return ErrorClassInvalid

	case statusCode == http.StatusUnauthorized, // 401
		statusCode == http.StatusForbidden: // 403
		return ErrorClassAuth

	case statusCode == http.StatusPaymentRequired: // 402
		return ErrorClassPayment

	case statusCode == http.StatusNotFound: // 404
		return ErrorClassNotFound

	case statusCode == http.StatusRequestTimeout: // 408
		return ErrorClassTransient

	case statusCode == 524: // Cloudflare timeout — origin didn't respond in time
		return ErrorClassTransient

	case statusCode == http.StatusTooManyRequests: // 429
		return ErrorClassRateLimit

	case statusCode == 529: // Anthropic overloaded — retry elsewhere without cooling down the account
		return ErrorClassTransient

	case statusCode >= 500 && statusCode <= 599:
		return ErrorClassTransient

	default:
		return ErrorClassFatal
	}
}

// isDeactivatedWorkspace checks the body for signs that the account is
// permanently dead (deactivated workspace, cancelled subscription, etc.).
func isDeactivatedWorkspace(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "deactivated_workspace") ||
		strings.Contains(s, "subscription") ||
		strings.Contains(s, "billing") ||
		strings.Contains(s, "payment_required")
}

// isClaudeOrganizationDisabled checks for Anthropic responses that indicate
// the org behind a Claude OAuth token has been disabled permanently.
func isClaudeOrganizationDisabled(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "organization has been disabled") ||
		strings.Contains(s, "organization is disabled") ||
		strings.Contains(s, "organization_has_been_disabled") ||
		strings.Contains(s, "organization_disabled") ||
		strings.Contains(s, "oauth authentication is currently not allowed")
}

// isCloudflareChallenge detects Cloudflare bot-mitigation responses that return
// 403 with an HTML challenge page. These are transient (not auth failures) and
// should not penalize accounts. Only the explicit Cf-Mitigated marker counts:
// chatgpt.com also serves its flagged-session 403 pages through Cloudflare, and
// those "Server: cloudflare + <html" responses are account-level rejections that
// must retire the connection, not be treated as transient challenges.
func isCloudflareChallenge(body []byte, headers http.Header) bool {
	return headers.Get("Cf-Mitigated") == "challenge"
}

// isOpenAIGatewayBlock detects a full HTML account-gate page served in place
// of an API error envelope (e.g. chatgpt.com serving a flagged-session or
// verification page). Unlike Cloudflare bot challenges (handled separately as
// transient), an upstream HTML page on an authenticated API call means the
// account itself was rejected, so the connection should be retired rather than
// handed a small penalty and retried forever.
func isOpenAIGatewayBlock(body []byte) bool {
	if !bytes.Contains(body, []byte("<html")) && !bytes.Contains(body, []byte("<!DOCTYPE html")) {
		return false
	}
	// Cloudflare challenges are classified transient by the caller before this
	// helper runs; do not treat them as account death here.
	return true
}

func isCyberPolicyError(body []byte) bool {
	// Only match the upstream's actual error envelope. Naive substring
	// matching trips on any assistant output that happens to contain the
	// literal phrase "cyber_policy" — e.g. an assistant explaining this
	// very codebase — which would silently replace its reply with a
	// synthetic refusal. Parse the JSON shape and require code or type
	// to equal the sentinel.
	if !bytes.Contains(body, []byte(`"cyber_policy"`)) {
		return false
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	var env struct {
		Type  string `json:"type"`
		Code  string `json:"code"`
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return false
	}
	return env.Code == "cyber_policy" || env.Type == "cyber_policy" ||
		env.Error.Code == "cyber_policy" || env.Error.Type == "cyber_policy"
}

func isCodexModelUnavailableError(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "model_not_found") ||
		strings.Contains(s, "model not found") ||
		strings.Contains(s, "model_not_supported") ||
		strings.Contains(s, "model not supported") ||
		strings.Contains(s, "model is not supported") ||
		strings.Contains(s, "not available for") ||
		strings.Contains(s, "does not have access to model")
}

// Retryable returns true if this class should be retried on another account.
func (c ErrorClass) Retryable() bool {
	switch c {
	case ErrorClassTransient, ErrorClassRateLimit, ErrorClassAuth,
		ErrorClassPayment, ErrorClassNotFound:
		return true
	default:
		return false
	}
}

// String returns a human-readable label.
func (c ErrorClass) String() string {
	switch c {
	case ErrorClassNone:
		return "none"
	case ErrorClassTransient:
		return "transient"
	case ErrorClassRateLimit:
		return "rate_limit"
	case ErrorClassAuth:
		return "auth"
	case ErrorClassPayment:
		return "payment"
	case ErrorClassNotFound:
		return "not_found"
	case ErrorClassInvalid:
		return "invalid"
	case ErrorClassFatal:
		return "fatal"
	default:
		return "unknown"
	}
}
