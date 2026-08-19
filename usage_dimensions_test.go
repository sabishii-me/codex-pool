package main

import "testing"

func TestProviderAwareCacheDiagnostics(t *testing.T) {
	exclusive := UsageDimension{ProviderID: string(AccountTypeDeepSeek), InputTokens: 10, CachedTokens: 90, CacheReadReporting: "complete"}
	applyUsageCacheDiagnostics(&exclusive)
	if exclusive.CacheSemantics != "exclusive" || exclusive.CacheReadSharePct == nil || *exclusive.CacheReadSharePct != 90 {
		t.Fatalf("exclusive diagnostics=%+v", exclusive)
	}
	inclusive := UsageDimension{ProviderID: string(AccountTypeCodex), InputTokens: 100, CachedTokens: 25, CacheReadReporting: "complete"}
	applyUsageCacheDiagnostics(&inclusive)
	if inclusive.CacheSemantics != "inclusive" || inclusive.CacheReadSharePct == nil || *inclusive.CacheReadSharePct != 25 {
		t.Fatalf("inclusive diagnostics=%+v", inclusive)
	}
	unknown := UsageDimension{ProviderID: "unregistered", InputTokens: 100, CachedTokens: 25, CacheReadReporting: "complete"}
	applyUsageCacheDiagnostics(&unknown)
	if unknown.CacheSemantics != "unknown" || unknown.CacheReadSharePct != nil || unknown.CacheDiagnostic == "" {
		t.Fatalf("unknown diagnostics=%+v", unknown)
	}
}

func TestCacheDiagnosticsDoNotFabricateZeroWhenUnreported(t *testing.T) {
	// Provider did NOT report cache telemetry: projection must expose no share
	// percentage (unavailable), never a fabricated 0% hit rate.
	unreported := UsageDimension{ProviderID: string(AccountTypeDeepSeek), InputTokens: 1000, OutputTokens: 100, CacheReadReporting: "none"}
	applyUsageCacheDiagnostics(&unreported)
	if unreported.CacheSemantics != "exclusive" {
		t.Fatalf("semantics=%q", unreported.CacheSemantics)
	}
	if unreported.CacheReadSharePct != nil {
		t.Fatalf("unreported cache must not present a share pct, got %v", *unreported.CacheReadSharePct)
	}
	if unreported.CacheReadReporting != "none" {
		t.Fatalf("reporting=%q, want none", unreported.CacheReadReporting)
	}
}

func TestCacheDiagnosticsKnownZeroStillReportsZero(t *testing.T) {
	// Provider explicitly reported cacheReadTokens=0: 0% is a real fact.
	knownZero := UsageDimension{ProviderID: string(AccountTypeDeepSeek), InputTokens: 1000, CachedTokens: 0, OutputTokens: 100, CacheReadReporting: "complete"}
	applyUsageCacheDiagnostics(&knownZero)
	if knownZero.CacheReadSharePct == nil || *knownZero.CacheReadSharePct != 0 {
		t.Fatalf("known-zero share=%v, want 0", knownZero.CacheReadSharePct)
	}
}

func TestCacheDiagnosticsPartialReporting(t *testing.T) {
	// Some requests reported cache, others did not: percentage must be exposed
	// (partial) but the caller can decide how to label coverage.
	partial := UsageDimension{ProviderID: string(AccountTypeCodex), InputTokens: 100, CachedTokens: 25, CacheReadReporting: "partial"}
	applyUsageCacheDiagnostics(&partial)
	if partial.CacheReadSharePct == nil || *partial.CacheReadSharePct != 25 {
		t.Fatalf("partial share=%v", partial.CacheReadSharePct)
	}
	if partial.CacheReadReporting != "partial" {
		t.Fatalf("reporting=%q", partial.CacheReadReporting)
	}
}

func TestReportingStateClassification(t *testing.T) {
	tests := []struct {
		reported, total int64
		want            string
	}{
		{0, 0, "none"},
		{0, 5, "none"},
		{5, 5, "complete"},
		{3, 5, "partial"},
		{5, 3, "complete"}, // defensive: reported > total treats as complete
	}
	for _, tt := range tests {
		if got := reportingState(tt.reported, tt.total); got != tt.want {
			t.Fatalf("reportingState(%d,%d)=%q, want %q", tt.reported, tt.total, got, tt.want)
		}
	}
}

func TestUsageCostDiagnosticsPreserveUnknownPricing(t *testing.T) {
	unknown := UsageDimension{ID: "glm-5.2", ProviderID: string(AccountTypeZAI), CostUSD: 0}
	applyUsageCostDiagnostics(&unknown, newPricingData(), nil)
	if unknown.CostStatus != "unknown" || unknown.CostReason == "" {
		t.Fatalf("unknown cost diagnostics=%+v", unknown)
	}
}
