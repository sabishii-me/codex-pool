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

func TestAccessPolicyOpenSessionStillHonorsActiveBan(t *testing.T) {
	attempts := &accessAttemptStub{banned: true}
	policy := &AccessPolicy{openSessionAccess: true, attempts: attempts}
	response := httptest.NewRecorder()
	if policy.RequireSession(response, httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("banned request authenticated")
	}
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestAccessPolicyOpenSessionDoesNotRequireIdentity(t *testing.T) {
	policy := &AccessPolicy{openSessionAccess: true}
	if !policy.RequireSession(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("open local deployment rejected request")
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
	response = httptest.NewRecorder()
	if policy.RequireAdmin(response, httptest.NewRequest(http.MethodGet, "/", nil)) || response.Code != http.StatusUnauthorized {
		t.Fatalf("non-elevated status=%d", response.Code)
	}

	elevated = true
	if !policy.RequireAdmin(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)) || attempts.success != 1 {
		t.Fatalf("elevated admin rejected, successes=%d", attempts.success)
	}
}
