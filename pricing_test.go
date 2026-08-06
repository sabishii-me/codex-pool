package main

import (
	"testing"
	"time"
)

func TestCodexProLiteSubscriptionCost(t *testing.T) {
	t.Parallel()

	for _, plan := range []string{"prolite", "PROLITE", " ProLite "} {
		monthly, label := getSubscriptionCost(AccountTypeCodex, plan)
		if monthly != 100 || label != "Codex Pro Lite" {
			t.Fatalf("getSubscriptionCost(codex, %q) = (%v, %q), want (100, %q)", plan, monthly, label, "Codex Pro Lite")
		}
	}
}

func TestEstimateSubscriptionSpendUsesObservedBillingCycles(t *testing.T) {
	now := time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		firstSeen  time.Time
		wantSpend  float64
		wantCycles int
	}{
		{name: "current cycle", firstSeen: now, wantSpend: 200, wantCycles: 1},
		{name: "before first renewal", firstSeen: now.Add(-29 * 24 * time.Hour), wantSpend: 200, wantCycles: 1},
		{name: "first renewal", firstSeen: now.Add(-30 * 24 * time.Hour), wantSpend: 400, wantCycles: 2},
		{name: "six observed cycles", firstSeen: now.Add(-171 * 24 * time.Hour), wantSpend: 1200, wantCycles: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spend, cycles := estimateSubscriptionSpend(200, tt.firstSeen, now)
			if spend != tt.wantSpend || cycles != tt.wantCycles {
				t.Fatalf("estimateSubscriptionSpend() = (%v, %d), want (%v, %d)", spend, cycles, tt.wantSpend, tt.wantCycles)
			}
		})
	}
}

func TestAccountPlanForSubscriptionPreservesProLite(t *testing.T) {
	t.Parallel()

	for _, plan := range []string{"prolite", "PROLITE", "Codex ProLite"} {
		if got := accountPlanForSubscription(plan); got != "prolite" {
			t.Fatalf("accountPlanForSubscription(%q) = %q, want prolite", plan, got)
		}
	}
}
