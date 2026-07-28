package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestUsageDimensionsPreserveModelProviderAndConnectionGrain(t *testing.T) {
	store, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for index, usage := range []RequestUsage{
		{RequestID: "one", Timestamp: time.Now(), UserID: "member", ProviderID: AccountTypeCodex, ConnectionID: "codex-1", Model: "gpt-5.6-sol", InputTokens: 10, OutputTokens: 4, BillableTokens: 14},
		{RequestID: "two", Timestamp: time.Now(), UserID: "member", ProviderID: AccountTypeDeepSeek, ConnectionID: "deepseek-1", Model: "deepseek-v4-pro", InputTokens: 20, CachedInputTokens: 5, OutputTokens: 6, BillableTokens: 21},
	} {
		usage.Timestamp = usage.Timestamp.Add(time.Duration(index) * time.Second)
		if _, err := store.recordUsageEvent(usageEventFromRequest(usage, float64(index+1))); err != nil {
			t.Fatal(err)
		}
	}
	models, providers, connections, err := store.getUsageDimensions("member", 7*24, true)
	if err != nil {
		t.Fatal(err)
	}
	modelHourly, err := store.getUsageModelHourly("member", 24)
	if err != nil {
		t.Fatal(err)
	}
	hourly, err := store.getUsageHourly("member", 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || len(providers) != 2 || len(connections) != 2 || len(modelHourly) != 2 || len(hourly) != 2 {
		t.Fatalf("models=%#v providers=%#v connections=%#v modelHourly=%#v hourly=%#v", models, providers, connections, modelHourly, hourly)
	}
	if totals := usageTotalsFromHourly(hourly); totals.TotalBillableTokens != 35 || totals.TotalInputTokens != 30 || totals.TotalCachedTokens != 5 || totals.TotalOutputTokens != 10 || totals.RequestCount != 2 {
		t.Fatalf("hourly totals=%#v", totals)
	}
	if models[0].BillableTokens+models[1].BillableTokens != 35 {
		t.Fatalf("models=%#v", models)
	}
}

func TestUsageRangeStartUsesCompleteUTCHourBuckets(t *testing.T) {
	now := time.Date(2026, time.July, 27, 23, 45, 0, 0, time.UTC)
	if got := usageRangeStart(now, 24); !got.Equal(time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("24 hour range starts at %s", got)
	}
}

func TestUsageProjectionTotalsAreLimitedToReturnedRange(t *testing.T) {
	totals := usageTotalsFromHourly([]UserHourlyUsage{
		{InputTokens: 10, CachedTokens: 3, OutputTokens: 4, ReasoningTokens: 2, BillableTokens: 14, RequestCount: 1},
		{InputTokens: 20, CachedTokens: 5, OutputTokens: 6, ReasoningTokens: 1, BillableTokens: 26, RequestCount: 2},
	})
	if totals.TotalInputTokens != 30 || totals.TotalCachedTokens != 8 || totals.TotalOutputTokens != 10 || totals.TotalReasoningTokens != 3 || totals.TotalBillableTokens != 40 || totals.RequestCount != 3 {
		t.Fatalf("range totals = %#v", totals)
	}
}

func TestUsageProjectionRangeBounds(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v2/usage?hours=99999&days=999", nil)
	hours, days := parseUsageRange(r)
	if hours != 24*31 || days != 90 {
		t.Fatalf("hours=%d days=%d", hours, days)
	}
}

func TestDataAPIRoutesScopedUsageProjections(t *testing.T) {
	called := ""
	api := &DataAPI{
		usageV2:          func(http.ResponseWriter, *http.Request) { called = "usage" },
		usageEconomicsV2: func(http.ResponseWriter, *http.Request) { called = "economics" },
	}
	if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v2/usage?scope=me", nil)) || called != "usage" {
		t.Fatalf("usage called=%q", called)
	}
	called = ""
	if !api.TryServe(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v2/usage/economics?scope=pool", nil)) || called != "economics" {
		t.Fatalf("economics called=%q", called)
	}
}
