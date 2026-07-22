package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCanonicalPersistenceFailurePreventsProjectionMutation(t *testing.T) {
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := analytics.db.Close(); err != nil {
		t.Fatal(err)
	}
	account := &Account{ID: "account", Type: AccountTypeDeepSeek}
	handler := &proxyHandler{cfg: &config{}, analyticsStore: analytics, recent: newRecentErrors(5)}
	handler.recordUsage(account, RequestUsage{Timestamp: time.Now(), RequestID: "request", AccountID: account.ID, AccountType: account.Type, InputTokens: 10, BillableTokens: 10})
	account.mu.Lock()
	count := account.Totals.RequestCount
	account.mu.Unlock()
	if count != 0 {
		t.Fatalf("in-memory request count = %d after persistence failure, want 0", count)
	}
}

func TestCanonicalUsageRejectsMissingRequestIdentity(t *testing.T) {
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.db.Close()
	account := &Account{ID: "account", Type: AccountTypeDeepSeek}
	handler := &proxyHandler{cfg: &config{}, analyticsStore: analytics, recent: newRecentErrors(5)}
	handler.recordUsage(account, RequestUsage{Timestamp: time.Now(), AccountID: account.ID, AccountType: account.Type, InputTokens: 10, BillableTokens: 10})
	account.mu.Lock()
	count := account.Totals.RequestCount
	account.mu.Unlock()
	if count != 0 {
		t.Fatalf("in-memory request count = %d, want 0", count)
	}
	var events int
	if err := analytics.db.QueryRow(`SELECT COUNT(*) FROM usage_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatalf("canonical events = %d, want 0", events)
	}
}

func TestRecordUsageDeduplicatesBeforeInMemoryAndAnalyticsProjections(t *testing.T) {
	usageStore, err := newUsageStore(filepath.Join(t.TempDir(), "usage.db"), 30)
	if err != nil {
		t.Fatal(err)
	}
	defer usageStore.Close()
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.db.Close()
	account := &Account{ID: "account", Type: AccountTypeDeepSeek, PlanType: "plan"}
	handler := &proxyHandler{cfg: &config{}, store: usageStore, analyticsStore: analytics, recent: newRecentErrors(5)}
	usage := RequestUsage{
		Timestamp: time.Now(), AccountID: account.ID, AccountType: account.Type,
		RequestID: "request-1", InputTokens: 100, CachedInputTokens: 20,
		CacheCreationTokens: 10, OutputTokens: 30, ReasoningTokens: 5, BillableTokens: 100,
	}
	handler.recordUsage(account, usage)
	usage.Timestamp = usage.Timestamp.Add(time.Second)
	handler.recordUsage(account, usage)
	account.mu.Lock()
	totals := account.Totals
	account.mu.Unlock()
	if totals.RequestCount != 1 || totals.TotalInputTokens != 100 || totals.TotalOutputTokens != 30 {
		t.Fatalf("in-memory totals duplicated: %+v", totals)
	}
	var analyticsCount int64
	if err := analytics.db.QueryRow(`SELECT COUNT(*) FROM request_costs WHERE account_id = ?`, account.ID).Scan(&analyticsCount); err != nil {
		t.Fatal(err)
	}
	if analyticsCount != 1 {
		t.Fatalf("analytics requests = %d, want 1", analyticsCount)
	}
}
