package main

import (
	"testing"
	"time"
)

func TestParseGeminiRateLimitResetDelay(t *testing.T) {
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	body := []byte(`{"error":{"message":"Resource exhausted","details":[{"metadata":{"quotaResetDelay":"12.345s"}}]}}`)
	reset, ok := parseGeminiRateLimitReset(body, now)
	if !ok || !reset.Equal(now.Add(13*time.Second)) {
		t.Fatalf("reset = %v, ok = %v", reset, ok)
	}
}

func TestParseGeminiRateLimitRetryText(t *testing.T) {
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	reset, ok := parseGeminiRateLimitReset([]byte(`{"error":{"message":"Please retry in 2.1s"}}`), now)
	if !ok || !reset.Equal(now.Add(3*time.Second)) {
		t.Fatalf("reset = %v, ok = %v", reset, ok)
	}
}

func TestParseZAIRateLimitResetUsesProviderTimezone(t *testing.T) {
	now := time.Date(2026, 7, 22, 13, 30, 0, 0, time.UTC)
	body := []byte(`{"type":"error","error":{"type":"rate_limit_error","code":"1308","message":"[1308][Usage limit reached for 5 hour. Your limit will reset at 2026-07-23 01:02:10][request]"}}`)
	reset, ok := parseZAIRateLimitReset(body, now)
	if !ok {
		t.Fatal("expected Z.ai reset timestamp")
	}
	want := time.Date(2026, 7, 22, 17, 2, 10, 0, time.UTC)
	if !reset.Equal(want) {
		t.Fatalf("reset=%v, want %v", reset, want)
	}
}

func TestZAIRateLimitResponseMakesConnectionUnavailableUntilReset(t *testing.T) {
	now := time.Now()
	resetLocal := now.In(time.FixedZone("ZAI-CST", 8*60*60)).Add(time.Hour)
	body := []byte(`{"error":{"message":"Usage limit reached for 5 hour. Your limit will reset at ` + resetLocal.Format("2006-01-02 15:04:05") + `"}}`)
	connection := &ProviderConnection{Type: AccountTypeZAI, ID: "zai-limited"}
	handler := &proxyHandler{cfg: &config{}}
	if wait := handler.applyRateLimitResponse(connection, nil, body); wait < 59*time.Minute {
		t.Fatalf("cooldown=%v", wait)
	}
	connection.mu.Lock()
	available := accountAvailableForRoutingLocked(connection, time.Now())
	connection.mu.Unlock()
	if available {
		t.Fatal("quota-exhausted Z.ai connection remained available")
	}
}

func TestParseGeminiDailyLimitUsesPacificMidnight(t *testing.T) {
	now := time.Date(2026, 7, 15, 6, 30, 0, 0, time.UTC)
	reset, ok := parseGeminiRateLimitReset([]byte(`{"error":{"message":"Requests per day quota exhausted"}}`), now)
	if !ok {
		t.Fatal("expected reset")
	}
	want := time.Date(2026, 7, 15, 7, 0, 0, 0, time.UTC)
	if !reset.Equal(want) {
		t.Fatalf("reset = %v, want %v", reset, want)
	}
}
