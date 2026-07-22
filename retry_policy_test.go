package main

import (
	"context"
	"testing"
	"time"
)

func TestRetryPolicyCalculatesBoundedAttempts(t *testing.T) {
	cases := []struct {
		configured, providerConnections, totalConnections, want int
	}{
		{configured: 0, providerConnections: 0, totalConnections: 0, want: 1},
		{configured: 3, providerConnections: 1, totalConnections: 10, want: 3},
		{configured: 1, providerConnections: 4, totalConnections: 10, want: 4},
		{configured: 10, providerConnections: 4, totalConnections: 5, want: 5},
	}
	for _, tc := range cases {
		policy := RetryPolicy{ConfiguredAttempts: tc.configured}
		if got := policy.Attempts(tc.providerConnections, tc.totalConnections); got != tc.want {
			t.Errorf("Attempts(%d,%d,%d)=%d, want %d", tc.configured, tc.providerConnections, tc.totalConnections, got, tc.want)
		}
	}
}

func TestRetryPolicyCapsCooldownAndHonorsCancellation(t *testing.T) {
	policy := RetryPolicy{MaxCooldownWait: 10 * time.Second}
	if got := policy.CooldownWait(time.Minute); got != 10*time.Second {
		t.Fatalf("capped cooldown=%v", got)
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
