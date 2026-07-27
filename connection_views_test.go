package main

import (
	"testing"
	"time"
)

func TestPoolStatsConnectionsReturnsDetachedReadModels(t *testing.T) {
	connection := &ProviderConnection{
		ID: "connection-1", Type: AccountTypeCodex,
		Identity:              ConnectionIdentity{DisplayName: "Primary", Attributes: map[string]string{"workspace": "alpha"}},
		PlanType:              "plus",
		Totals:                AccountUsage{TotalInputTokens: 100, TotalCachedTokens: 25},
		Usage:                 UsageSnapshot{PrimaryUsed: 0.4, primarySet: true},
		RateLimitResetCredits: []RateLimitResetCredit{{ExpiresAt: time.Now().Add(time.Hour)}},
	}
	service := NewConnectionViewService(newProviderPool([]*ProviderConnection{connection}))
	views := service.PoolStatsConnections(time.Now())
	if len(views) != 1 || views[0].ConnectionID != connection.ID || views[0].View.DisplayName != "Primary" {
		t.Fatalf("unexpected snapshot: %+v", views)
	}
	views[0].View.IdentityAttributes["workspace"] = "changed"
	views[0].View.ResetCreditExpirations[0] = "changed"
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.Identity.Attributes["workspace"] != "alpha" {
		t.Fatal("snapshot identity attributes alias mutable connection state")
	}
	if connection.RateLimitResetCredits[0].ExpiresAt.IsZero() {
		t.Fatal("snapshot reset-credit data mutated connection state")
	}
}

func TestOperatorConnectionsExposeProviderStateManagementCapability(t *testing.T) {
	connection := &ProviderConnection{
		ID: "codex", Type: AccountTypeCodex, ResetCreditsRetrievedAt: time.Now(), ResetCreditsAvailable: 1,
		RateLimitResetCredits: []RateLimitResetCredit{{ID: "private", ExpiresAt: time.Now().Add(time.Hour)}},
	}
	view := NewConnectionViewService(newProviderPool([]*ProviderConnection{connection})).OperatorConnections()[0]
	if !view.ResetCredits.ManagementAvailable || !view.ResetCredits.InventoryRefreshAvailable || !view.ResetCredits.RedemptionAvailable {
		t.Fatalf("reset-credit management projection = %#v", view.ResetCredits)
	}
}

func TestPoolStatsConnectionsMarksPrimaryAndCyberEligibility(t *testing.T) {
	now := time.Now()
	low := &ProviderConnection{ID: "low", Type: AccountTypeCodex, Penalty: 1}
	high := &ProviderConnection{ID: "high", Type: AccountTypeCodex, CyberAccess: true, ExpiresAt: now.Add(time.Hour)}
	views := NewConnectionViewService(newProviderPool([]*ProviderConnection{low, high})).PoolStatsConnections(now)
	if len(views) != 2 {
		t.Fatalf("len=%d", len(views))
	}
	primary, cyber := 0, 0
	for _, view := range views {
		if view.View.IsPrimary {
			primary++
		}
		if view.CyberEligible {
			cyber++
		}
	}
	if primary != 1 || cyber != 1 {
		t.Fatalf("primary=%d cyber=%d views=%+v", primary, cyber, views)
	}
}
