package main

import "testing"

func TestProviderAwareCacheDiagnostics(t *testing.T) {
	exclusive := UsageDimension{ProviderID: string(AccountTypeDeepSeek), InputTokens: 10, CachedTokens: 90}
	applyUsageCacheDiagnostics(&exclusive)
	if exclusive.CacheSemantics != "exclusive" || exclusive.CacheReadSharePct == nil || *exclusive.CacheReadSharePct != 90 {
		t.Fatalf("exclusive diagnostics=%+v", exclusive)
	}
	inclusive := UsageDimension{ProviderID: string(AccountTypeCodex), InputTokens: 100, CachedTokens: 25}
	applyUsageCacheDiagnostics(&inclusive)
	if inclusive.CacheSemantics != "inclusive" || inclusive.CacheReadSharePct == nil || *inclusive.CacheReadSharePct != 25 {
		t.Fatalf("inclusive diagnostics=%+v", inclusive)
	}
	unknown := UsageDimension{ProviderID: "unregistered", InputTokens: 100, CachedTokens: 25}
	applyUsageCacheDiagnostics(&unknown)
	if unknown.CacheSemantics != "unknown" || unknown.CacheReadSharePct != nil || unknown.CacheDiagnostic == "" {
		t.Fatalf("unknown diagnostics=%+v", unknown)
	}
}

func TestUsageCostDiagnosticsPreserveUnknownPricing(t *testing.T) {
	unknown := UsageDimension{ID: "glm-5.2", ProviderID: string(AccountTypeZAI), CostUSD: 0}
	applyUsageCostDiagnostics(&unknown, newPricingData())
	if unknown.CostStatus != "unknown" || unknown.CostReason == "" {
		t.Fatalf("unknown cost diagnostics=%+v", unknown)
	}
}
