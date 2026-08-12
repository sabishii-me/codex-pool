package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type accessAttemptStub struct {
	banned   bool
	failures int
	success  int
}

func (stub *accessAttemptStub) isBanned(string) bool { return stub.banned }
func (stub *accessAttemptStub) recordFailure(string) bool {
	stub.failures++
	return false
}
func (stub *accessAttemptStub) recordSuccess(string) { stub.success++ }

func TestAccessPolicyBanPrecedesAuthentication(t *testing.T) {
	attempts := &accessAttemptStub{banned: true}
	policy := &AccessPolicy{attempts: attempts}
	response := httptest.NewRecorder()
	if policy.RequireSession(response, httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("banned request authenticated")
	}
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestAccessPolicyAlwaysRequiresIdentity(t *testing.T) {
	policy := &AccessPolicy{}
	response := httptest.NewRecorder()
	if policy.RequireSession(response, httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("missing authentication opened a protected route")
	}
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestAccessPolicySessionTracksFailureAndSuccess(t *testing.T) {
	attempts := &accessAttemptStub{}
	identityAvailable := false
	policy := &AccessPolicy{
		resolveUser: func(*http.Request) (*GatewayUser, bool) {
			return &GatewayUser{ID: "member"}, identityAvailable
		},
		attempts: attempts,
	}
	response := httptest.NewRecorder()
	if policy.RequireSession(response, httptest.NewRequest(http.MethodGet, "/", nil)) || response.Code != http.StatusUnauthorized || attempts.failures != 1 {
		t.Fatalf("unauthorized status=%d failures=%d", response.Code, attempts.failures)
	}
	identityAvailable = true
	if !policy.RequireSession(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)) || attempts.success != 1 {
		t.Fatalf("authorized success count=%d", attempts.success)
	}
}

func TestAccessPolicyAdminRequiresAllowlistAndElevation(t *testing.T) {
	attempts := &accessAttemptStub{}
	user := &GatewayUser{ID: "operator", Email: "member@example.com"}
	elevated := false
	policy := &AccessPolicy{
		adminEmails: []string{"admin@example.com"},
		resolveUser: func(*http.Request) (*GatewayUser, bool) { return user, true },
		isElevated:  func(*http.Request, string) bool { return elevated },
		attempts:    attempts,
	}
	response := httptest.NewRecorder()
	if policy.RequireAdmin(response, httptest.NewRequest(http.MethodGet, "/", nil)) || response.Code != http.StatusForbidden || attempts.failures != 1 {
		t.Fatalf("non-admin status=%d failures=%d", response.Code, attempts.failures)
	}

	user.Email = "admin@example.com"
	// Reads (GET) and account additions (POST) require admin sign-in only.
	response = httptest.NewRecorder()
	if !policy.RequireAdmin(response, httptest.NewRequest(http.MethodPost, "/", nil)) {
		t.Fatalf("admin POST rejected without elevation, status=%d", response.Code)
	}
	// Destructive DELETE still requires MFA elevation.
	response = httptest.NewRecorder()
	if policy.RequireAdmin(response, httptest.NewRequest(http.MethodDelete, "/", nil)) || response.Code != http.StatusUnauthorized {
		t.Fatalf("non-elevated DELETE status=%d", response.Code)
	}

	elevated = true
	if !policy.RequireAdmin(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/", nil)) || attempts.success != 2 {
		t.Fatalf("elevated admin rejected, successes=%d", attempts.success)
	}
}
