package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
