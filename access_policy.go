package main

import "net/http"

type accessAttemptTracker interface {
	isBanned(ip string) bool
	recordFailure(ip string) bool
	recordSuccess(ip string)
}

// AccessPolicy owns route-level authentication and administrator elevation
// decisions. OAuth callbacks and session decoding remain separate concerns;
// this policy consumes only a resolved gateway identity.
type AccessPolicy struct {
	openSessionAccess bool
	adminEmails       []string
	resolveUser       func(*http.Request) (*GatewayUser, bool)
	isElevated        func(*http.Request, string) bool
	attempts          accessAttemptTracker
	clientIP          func(*http.Request) string
}

func (policy *AccessPolicy) RequireAdmin(w http.ResponseWriter, r *http.Request) bool {
	ip := policy.requestIP(r)
	if policy.attempts != nil && policy.attempts.isBanned(ip) {
		http.Error(w, "too many failed attempts, try again later", http.StatusTooManyRequests)
		return false
	}

	user, ok := policy.user(r)
	if !ok {
		http.Error(w, "sign in required", http.StatusUnauthorized)
		return false
	}
	if !adminEmailAllowed(policy.adminEmails, user.Email) {
		if policy.attempts != nil {
			policy.attempts.recordFailure(ip)
		}
		http.Error(w, "not an admin", http.StatusForbidden)
		return false
	}
	if policy.isElevated == nil || !policy.isElevated(r, user.ID) {
		http.Error(w, "two-factor verification required", http.StatusUnauthorized)
		return false
	}
	if policy.attempts != nil {
		policy.attempts.recordSuccess(ip)
	}
	return true
}

func (policy *AccessPolicy) RequireSession(w http.ResponseWriter, r *http.Request) bool {
	ip := policy.requestIP(r)
	if policy.attempts != nil && policy.attempts.isBanned(ip) {
		http.Error(w, "too many failed attempts, try again later", http.StatusTooManyRequests)
		return false
	}
	if policy.openSessionAccess {
		return true
	}
	if _, ok := policy.user(r); ok {
		if policy.attempts != nil {
			policy.attempts.recordSuccess(ip)
		}
		return true
	}
	if policy.attempts != nil {
		policy.attempts.recordFailure(ip)
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func (policy *AccessPolicy) user(r *http.Request) (*GatewayUser, bool) {
	if policy == nil || policy.resolveUser == nil {
		return nil, false
	}
	return policy.resolveUser(r)
}

func (policy *AccessPolicy) requestIP(r *http.Request) string {
	if policy != nil && policy.clientIP != nil {
		return policy.clientIP(r)
	}
	return getClientIP(r)
}

func (h *proxyHandler) accessPolicy() *AccessPolicy {
	if h.access != nil {
		return h.access
	}
	openSessionAccess := h.cfg == nil || h.cfg.oauthGoogleClientID == ""
	var adminEmails []string
	if h.cfg != nil {
		adminEmails = h.cfg.adminEmails
	}
	policy := &AccessPolicy{
		openSessionAccess: openSessionAccess,
		adminEmails:       adminEmails,
		resolveUser:       h.sessionUser,
		isElevated:        adminElevated,
		clientIP:          getClientIP,
	}
	if h.bruteForce != nil {
		policy.attempts = h.bruteForce
	}
	return policy
}

// Compatibility wrappers keep existing route and test contracts while access
// decisions live behind the focused policy boundary.
func (h *proxyHandler) checkAdminAuth(w http.ResponseWriter, r *http.Request) bool {
	return h.accessPolicy().RequireAdmin(w, r)
}

func (h *proxyHandler) checkAdminOrSessionAuth(w http.ResponseWriter, r *http.Request) bool {
	return h.accessPolicy().RequireSession(w, r)
}
