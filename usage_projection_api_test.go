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
	models, providers, connections, err := store.getUsageDimensions("member", 7, true)
	if err != nil {
		t.Fatal(err)
	}
	hourly, err := store.getUsageModelHourly("member", 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || len(providers) != 2 || len(connections) != 2 || len(hourly) != 2 {
		t.Fatalf("models=%#v providers=%#v connections=%#v hourly=%#v", models, providers, connections, hourly)
	}
	if models[0].BillableTokens+models[1].BillableTokens != 35 {
		t.Fatalf("models=%#v", models)
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
