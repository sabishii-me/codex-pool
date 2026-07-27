package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRetryPolicyCalculatesBoundedAttempts(t *testing.T) {
	cases := []struct {
		configured, providerConnections, totalConnections, want int
	}{
		{configured: 0, providerConnections: 0, totalConnections: 0, want: 1},
		{configured: 3, providerConnections: 1, totalConnections: 10, want: 1},
		{configured: 1, providerConnections: 4, totalConnections: 10, want: 1},
		{configured: 10, providerConnections: 4, totalConnections: 5, want: 4},
	}
	for _, tc := range cases {
		policy := RetryPolicy{ConfiguredAttempts: tc.configured}
		if got := policy.Attempts(tc.providerConnections, tc.totalConnections); got != tc.want {
			t.Errorf("Attempts(%d,%d,%d)=%d, want %d", tc.configured, tc.providerConnections, tc.totalConnections, got, tc.want)
		}
	}
}

func TestRetryPolicyRejectsLongCooldownWaitAndHonorsCancellation(t *testing.T) {
	policy := RetryPolicy{MaxCooldownWait: 10 * time.Second}
	if got := policy.CooldownWait(time.Minute); got != 0 {
		t.Fatalf("long cooldown wait=%v, want immediate rejection", got)
	}
	if got := policy.CooldownWait(2 * time.Second); got != 2*time.Second {
		t.Fatalf("short cooldown=%v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if policy.ShouldRotate(ErrorClassTransient, ctx) {
		t.Fatal("cancelled request rotated")
	}
	if policy.ShouldRotate(ErrorClassInvalid, context.Background()) {
		t.Fatal("invalid request rotated")
	}
	if !policy.ShouldRotate(ErrorClassRateLimit, context.Background()) {
		t.Fatal("rate limit did not rotate")
	}
}

func TestLongProviderCooldownReturns429WithoutSleeping(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "cooldown-secret")
	connection := &ProviderConnection{
		Type: AccountTypeZAI, ID: "zai-cooling", AccessToken: "unused", PlanType: "zai",
		RateLimitUntil: time.Now().Add(time.Hour),
	}
	base, err := url.Parse("https://upstream.invalid")
	if err != nil {
		t.Fatal(err)
	}
	handler := &proxyHandler{
		cfg:  &config{maxAttempts: 3, maxInMemoryBodyBytes: 1024 * 1024},
		pool: newProviderPool([]*ProviderConnection{connection}, false), registry: anthropicContractRegistry(base),
		metrics: newMetrics(), recent: newRecentErrors(5),
		retryPolicy: RetryPolicy{ConfiguredAttempts: 3, MaxCooldownWait: 10 * time.Second},
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"glm-5.2","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("cooldown-secret", "cooldown-user"))
	response := httptest.NewRecorder()

	started := time.Now()
	handler.proxyRequest(response, request, "cooldown-contract")
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("long cooldown response took %v", elapsed)
	}
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	seconds, err := strconv.Atoi(response.Header().Get("Retry-After"))
	if err != nil || seconds < 3500 {
		t.Fatalf("Retry-After=%q, err=%v", response.Header().Get("Retry-After"), err)
	}
	if !strings.Contains(response.Body.String(), "provider connections are rate limited") {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestRetryPolicyControlsBufferedCyberRetry(t *testing.T) {
	policy := RetryPolicy{}
	if !policy.ShouldRetryBufferedCyberPolicy(true, 1, 2, false) {
		t.Fatal("eligible cyber retry rejected")
	}
	for _, allowed := range []bool{
		policy.ShouldRetryBufferedCyberPolicy(false, 1, 2, false),
		policy.ShouldRetryBufferedCyberPolicy(true, 2, 2, false),
		policy.ShouldRetryBufferedCyberPolicy(true, 1, 2, true),
	} {
		if allowed {
			t.Fatal("ineligible cyber retry accepted")
		}
	}
}
