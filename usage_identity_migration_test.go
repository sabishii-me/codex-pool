package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestRequestUsageCanonicalIdentityNormalizesBothDirections(t *testing.T) {
	legacy := RequestUsage{AccountID: "connection", AccountType: AccountTypeClaude}.canonicalIdentity()
	if legacy.ConnectionID != "connection" || legacy.ProviderID != AccountTypeClaude {
		t.Fatalf("legacy normalization = %#v", legacy)
	}
	canonical := RequestUsage{ConnectionID: "connection", ProviderID: AccountTypeGemini}.canonicalIdentity()
	if canonical.AccountID != "connection" || canonical.AccountType != AccountTypeGemini {
		t.Fatalf("canonical compatibility normalization = %#v", canonical)
	}
}

func TestRequestUsageDecodesLegacyPersistedIdentity(t *testing.T) {
	var usage RequestUsage
	if err := json.Unmarshal([]byte(`{"Timestamp":"2026-07-22T00:00:00Z","account_id":"legacy-connection","account_type":"deepseek","InputTokens":10}`), &usage); err != nil {
		t.Fatal(err)
	}
	usage = usage.canonicalIdentity()
	if usage.ConnectionID != "legacy-connection" || usage.ProviderID != AccountTypeDeepSeek {
		t.Fatalf("decoded legacy identity = %#v", usage)
	}
}

func TestCanonicalRequestUsagePersistsThroughBothStores(t *testing.T) {
	usage := RequestUsage{
		Timestamp: time.Now(), RequestID: "canonical-request", ConnectionID: "connection",
		ProviderID: AccountTypeNvidia, UserID: "user", InputTokens: 10, OutputTokens: 5, BillableTokens: 15,
	}
	bolt, err := newUsageStore(filepath.Join(t.TempDir(), "usage.db"), 30)
	if err != nil {
		t.Fatal(err)
	}
	defer bolt.Close()
	if recorded, err := bolt.recordIfNew(usage); err != nil || !recorded {
		t.Fatalf("Bolt record=%v err=%v", recorded, err)
	}
	totals, err := bolt.loadAccountUsage("connection")
	if err != nil || totals.RequestCount != 1 || totals.TotalBillableTokens != 15 {
		t.Fatalf("Bolt totals=%#v err=%v", totals, err)
	}
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.db.Close()
	if recorded, err := analytics.recordUsageEvent(usageEventFromRequest(usage, 0)); err != nil || !recorded {
		t.Fatalf("SQLite record=%v err=%v", recorded, err)
	}
	var providerID, connectionID string
	if err := analytics.db.QueryRow(`SELECT provider_id, connection_id FROM usage_events WHERE request_id = ?`, usage.RequestID).Scan(&providerID, &connectionID); err != nil {
		t.Fatal(err)
	}
	if providerID != "nvidia" || connectionID != "connection" {
		t.Fatalf("SQLite identity provider=%q connection=%q", providerID, connectionID)
	}
}
