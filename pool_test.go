package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScoreDoesNotRewardGlobalRecentUse(t *testing.T) {
	now := time.Now()
	recent := &Account{Type: AccountTypeCodex, PlanType: "plus", LastUsed: now.Add(-time.Minute), Usage: UsageSnapshot{SecondaryUsedPercent: 0.2, secondarySet: true}}
	idle := &Account{Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0.2, secondarySet: true}}
	if got, want := scoreAccountLocked(recent, now), scoreAccountLocked(idle, now); got != want {
		t.Fatalf("recent unbound score=%v, idle=%v; cache locality belongs to typed affinity", got, want)
	}
}

func TestScorePrefersHeadroomAndPlan(t *testing.T) {
	now := time.Now()
	pro := &Account{PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.2, SecondaryUsedPercent: 0.2}}
	plus := &Account{PlanType: "plus", Usage: UsageSnapshot{PrimaryUsedPercent: 0.1, SecondaryUsedPercent: 0.1}, Penalty: 0.5}

	if scoreAccount(pro, now) <= scoreAccount(plus, now) {
		t.Fatalf("expected pro with headroom to win")
	}
}

func TestScoreBoostsEmptyPrimaryWindow(t *testing.T) {
	now := time.Now()

	// Account with 76% 7d used but 0% 5h used, 4h until primary reset, 40h until secondary reset.
	// Should score much higher than base headroom (0.24) because the 5h window is wide open.
	highWeeklyEmptyPrimary := &Account{
		PlanType: "max",
		Usage: UsageSnapshot{
			PrimaryUsedPercent:   0.0,
			SecondaryUsedPercent: 0.76,
			PrimaryResetAt:       now.Add(4*time.Hour + 6*time.Minute),
			SecondaryResetAt:     now.Add(40 * time.Hour),
		},
	}
	score := scoreAccount(highWeeklyEmptyPrimary, now)
	// With the primary pace bonus, score should be well above 0.5
	if score < 0.5 {
		t.Fatalf("expected score > 0.5 for empty primary window, got %.2f", score)
	}

	// Compare: same 7d usage but primary window is also heavily used. Should score lower.
	highWeeklyHighPrimary := &Account{
		PlanType: "max",
		Usage: UsageSnapshot{
			PrimaryUsedPercent:   0.70,
			SecondaryUsedPercent: 0.76,
			PrimaryResetAt:       now.Add(4*time.Hour + 6*time.Minute),
			SecondaryResetAt:     now.Add(40 * time.Hour),
		},
	}
	scoreBusy := scoreAccount(highWeeklyHighPrimary, now)
	if score <= scoreBusy {
		t.Fatalf("empty primary (%.2f) should outscore busy primary (%.2f)", score, scoreBusy)
	}
}

func TestPenaltyDecay(t *testing.T) {
	now := time.Now()
	a := &Account{Penalty: 1.0, LastPenalty: now.Add(-10 * time.Minute)}
	scoreAccount(a, now)
	if a.Penalty >= 1.0 {
		t.Fatalf("penalty should decay")
	}
}

func TestProviderPoolReplaceClearsLocalAffinity(t *testing.T) {
	pool := newProviderPool(nil, false)
	if !pool.bindAffinity("aff", "account") {
		t.Fatal("expected affinity assignment")
	}
	pool.replace(nil)
	if len(pool.convPin) != 0 {
		t.Fatalf("reload retained local affinity: %+v", pool.convPin)
	}
}

func TestBindAffinityReassignsAtomically(t *testing.T) {
	pool := newProviderPool(nil, false)
	if !pool.bindAffinity("aff", "first") || !pool.bindAffinity("aff", "second") {
		t.Fatal("expected affinity assignment")
	}
	if binding := pool.convPin["aff"]; binding.AccountID != "second" || binding.TouchedAt.IsZero() {
		t.Fatalf("atomic rebind=%+v", binding)
	}
}

func TestBindAffinityBoundsStore(t *testing.T) {
	if providerAffinityMaxEntries > 20000 {
		t.Skip("store bound is too large for a unit allocation test")
	}
	pool := newProviderPool(nil, false)
	if pool.bindAffinity("", "account") || pool.bindAffinity("aff", "") {
		t.Fatal("empty affinity assignment should be rejected")
	}
	pool.convPin["expired"] = affinityBinding{AccountID: "account", TouchedAt: time.Now().Add(-providerAffinityTTL - time.Minute)}
	for index := 0; index < providerAffinityMaxEntries+10; index++ {
		if !pool.bindAffinity(fmt.Sprintf("aff-%d", index), "account") {
			t.Fatalf("affinity %d was rejected", index)
		}
	}
	if got := len(pool.convPin); got != providerAffinityMaxEntries {
		t.Fatalf("affinity entries=%d, want bounded %d", got, providerAffinityMaxEntries)
	}
	if _, exists := pool.convPin["expired"]; exists {
		t.Fatal("bounded-store cleanup retained expired affinity")
	}
}

func TestCandidateBreaksDeletedAffinityOwner(t *testing.T) {
	healthy := &Account{ID: "healthy", Type: AccountTypeCodex, PlanType: "plus"}
	pool := newProviderPool([]*Account{healthy}, false)
	pool.bindAffinity("aff", "deleted")
	if got := pool.candidate("aff", nil, AccountTypeCodex, "", ""); got != healthy {
		t.Fatalf("candidate=%v, want healthy after deleted owner", got)
	}
	if binding, exists := pool.convPin["aff"]; exists && binding.AccountID != healthy.ID {
		t.Fatalf("deleted-owner affinity was not safely rebound: %+v", binding)
	}
}

func TestCandidateBreaksDeadAffinity(t *testing.T) {
	dead := &Account{ID: "dead", Type: AccountTypeCodex, PlanType: "plus", Dead: true}
	healthy := &Account{ID: "healthy", Type: AccountTypeCodex, PlanType: "plus"}
	pool := newProviderPool([]*Account{dead, healthy}, false)
	pool.bindAffinity("aff", dead.ID)
	if got := pool.candidate("aff", nil, AccountTypeCodex, "", ""); got != healthy {
		t.Fatalf("candidate=%v, want healthy after dead binding", got)
	}
	if binding, exists := pool.convPin["aff"]; exists && binding.AccountID == dead.ID {
		t.Fatalf("dead affinity was not broken: %+v", binding)
	}
}

func TestCandidateBreaksDisabledAffinity(t *testing.T) {
	disabled := &Account{ID: "disabled", Type: AccountTypeCodex, PlanType: "plus", Disabled: true}
	healthy := &Account{ID: "healthy", Type: AccountTypeCodex, PlanType: "plus"}
	pool := newProviderPool([]*Account{disabled, healthy}, false)
	pool.bindAffinity("aff", disabled.ID)
	if got := pool.candidate("aff", nil, AccountTypeCodex, "", ""); got != healthy {
		t.Fatalf("candidate=%v, want healthy after disabled binding", got)
	}
	if binding, exists := pool.convPin["aff"]; exists && binding.AccountID != healthy.ID {
		t.Fatalf("disabled affinity was not safely rebound: %+v", binding)
	}
}

func TestCandidateExpiresStaleAffinity(t *testing.T) {
	stale := &Account{ID: "stale", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{PrimaryUsedPercent: 0.6, primarySet: true}}
	fresh := &Account{ID: "fresh", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{PrimaryUsedPercent: 0.01, primarySet: true}}
	pool := newProviderPool([]*Account{stale, fresh}, false)
	pool.convPin["aff"] = affinityBinding{AccountID: stale.ID, TouchedAt: time.Now().Add(-providerAffinityTTL - time.Minute)}

	if got := pool.candidate("aff", nil, AccountTypeCodex, "", ""); got != fresh {
		t.Fatalf("candidate=%v, want fresh after stale affinity expires", got)
	}
	if binding, exists := pool.convPin["aff"]; exists && binding.AccountID != fresh.ID {
		t.Fatalf("expired affinity was not safely rebound: %+v", binding)
	}
}

func TestCandidateUsesAffinityUnlessExcluded(t *testing.T) {
	a1 := &Account{ID: "a1", Type: AccountTypeCodex, Usage: UsageSnapshot{PrimaryUsedPercent: 0.1}}
	a2 := &Account{ID: "a2", Type: AccountTypeCodex, Usage: UsageSnapshot{PrimaryUsedPercent: 0.2}}
	p := newProviderPool([]*Account{a1, a2}, true)
	p.bindAffinity("c1", "a1")

	if got := p.candidate("c1", nil, "", "", ""); got == nil || got.ID != "a1" {
		t.Fatalf("expected bound a1, got %+v", got)
	}
	if got := p.candidate("c1", map[string]bool{"a1": true}, "", "", ""); got == nil || got.ID != "a2" {
		t.Fatalf("expected a2 when bound account excluded, got %+v", got)
	}
	if binding := p.convPin["c1"]; binding.AccountID != "a1" {
		t.Fatalf("request-local retry exclusion destroyed affinity: %+v", binding)
	}
}

func TestCandidateSkipsDeadOrDisabled(t *testing.T) {
	dead := &Account{ID: "dead", Type: AccountTypeCodex, Dead: true, Usage: UsageSnapshot{PrimaryUsedPercent: 0.0}}
	disabled := &Account{ID: "disabled", Type: AccountTypeCodex, Disabled: true, Usage: UsageSnapshot{PrimaryUsedPercent: 0.0}}
	ok := &Account{ID: "ok", Type: AccountTypeCodex, Usage: UsageSnapshot{PrimaryUsedPercent: 0.5}}
	p := newProviderPool([]*Account{dead, disabled, ok}, false)

	got := p.candidate("", nil, "", "", "")
	if got == nil || got.ID != "ok" {
		t.Fatalf("expected ok, got %+v", got)
	}
}

func TestCandidateSkipsRateLimitedCodexAccount(t *testing.T) {
	rateLimited := &Account{
		ID:             "limited",
		Type:           AccountTypeCodex,
		PlanType:       "pro",
		RateLimitUntil: time.Now().Add(time.Hour),
	}
	healthy := &Account{ID: "healthy", Type: AccountTypeCodex, PlanType: "pro"}
	pool := newProviderPool([]*Account{rateLimited, healthy}, false)

	if got := pool.candidate("", nil, AccountTypeCodex, "", ""); got != healthy {
		t.Fatalf("candidate = %v, want healthy account", got)
	}
}

func TestCandidateBreaksHardQuotaAffinity(t *testing.T) {
	exhausted := &Account{ID: "exhausted", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 1, secondarySet: true}}
	healthy := &Account{ID: "healthy", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0.1, secondarySet: true}}
	pool := newProviderPool([]*Account{exhausted, healthy}, false)
	pool.bindAffinity("aff", exhausted.ID)
	if got := pool.candidate("aff", nil, AccountTypeCodex, "", ""); got != healthy {
		t.Fatalf("candidate=%v, want healthy after hard quota", got)
	}
	if binding, exists := pool.convPin["aff"]; exists && binding.AccountID == exhausted.ID {
		t.Fatalf("hard-quota affinity was not broken: %+v", binding)
	}
}

func TestCandidateBreaksRateLimitedCodexAffinity(t *testing.T) {
	rateLimited := &Account{
		ID:             "limited",
		Type:           AccountTypeCodex,
		PlanType:       "pro",
		RateLimitUntil: time.Now().Add(time.Hour),
	}
	healthy := &Account{ID: "healthy", Type: AccountTypeCodex, PlanType: "pro"}
	pool := newProviderPool([]*Account{rateLimited, healthy}, false)
	pool.bindAffinity("conversation", rateLimited.ID)

	if got := pool.candidate("conversation", nil, AccountTypeCodex, "", ""); got != healthy {
		t.Fatalf("candidate = %v, want healthy account", got)
	}
	if binding, exists := pool.convPin["conversation"]; exists && binding.AccountID == rateLimited.ID {
		t.Fatalf("cooldown affinity was not broken: %+v", binding)
	}
}

func TestCandidateRequiredPlanFiltersAccounts(t *testing.T) {
	plus := &Account{ID: "plus", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{PrimaryUsedPercent: 0.1}}
	pro := &Account{ID: "pro", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.2}}
	p := newProviderPool([]*Account{plus, pro}, false)

	got := p.candidate("", nil, AccountTypeCodex, "pro", "")
	if got == nil || got.ID != "pro" {
		t.Fatalf("expected pro account, got %+v", got)
	}
}

func TestCodexPlansShareOrdinaryCapacityTier(t *testing.T) {
	for _, plan := range []string{"plus", "pro", "prolite", "PROLITE", " ProLite "} {
		if got := accountTier(AccountTypeCodex, plan); got != 2 {
			t.Fatalf("accountTier(codex, %q) = %d, want ordinary capacity tier 2", plan, got)
		}
	}
	for _, plan := range []string{"prolite", "PROLITE", " ProLite "} {
		if !isCodexProAccessPlan(plan) {
			t.Fatalf("expected %q to have Codex Pro access", plan)
		}
		if !planMatchesRequired(plan, "pro") {
			t.Fatalf("expected %q to satisfy a Pro plan requirement", plan)
		}
	}
}

func TestCandidateUsesCodexProLiteAlongsidePro(t *testing.T) {
	pro := &Account{ID: "pro", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.7, SecondaryUsedPercent: 0.7}}
	proLite := &Account{ID: "prolite", Type: AccountTypeCodex, PlanType: "prolite", Usage: UsageSnapshot{PrimaryUsedPercent: 0.1, SecondaryUsedPercent: 0.1}}
	p := newProviderPool([]*Account{pro, proLite}, false)

	got := p.candidate("", nil, AccountTypeCodex, "pro", "")
	if got == nil || got.ID != "prolite" {
		t.Fatalf("expected lower-usage Pro Lite account to compete with Pro, got %+v", got)
	}
}

func TestCandidateKeepsBoundCodexProLite(t *testing.T) {
	pro := &Account{ID: "pro", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.1, SecondaryUsedPercent: 0.1}}
	proLite := &Account{ID: "prolite", Type: AccountTypeCodex, PlanType: "prolite", Usage: UsageSnapshot{PrimaryUsedPercent: 0.5, SecondaryUsedPercent: 0.5}}
	p := newProviderPool([]*Account{pro, proLite}, false)
	p.bindAffinity("conversation", proLite.ID)

	got := p.candidate("conversation", nil, AccountTypeCodex, "pro", "")
	if got == nil || got.ID != "prolite" {
		t.Fatalf("expected bound Pro Lite account to remain eligible, got %+v", got)
	}
}

func TestCandidatePrefersClaudeMaxOverPro(t *testing.T) {
	// Claude pro should be tier 3 (last resort), max should be tier 1
	maxAcc := &Account{ID: "max1", Type: AccountTypeClaude, PlanType: "max", Usage: UsageSnapshot{
		PrimaryUsedPercent: 0.5, SecondaryUsedPercent: 0.7,
	}}
	proAcc := &Account{ID: "pro1", Type: AccountTypeClaude, PlanType: "pro", Usage: UsageSnapshot{
		PrimaryUsedPercent: 0.0, SecondaryUsedPercent: 0.1,
	}}
	p := newProviderPool([]*Account{proAcc, maxAcc}, false)

	got := p.candidate("", nil, AccountTypeClaude, "", "")
	if got == nil || got.ID != "max1" {
		t.Fatalf("expected max account preferred over pro even with worse usage, got %+v", got)
	}
}

func TestCandidateFallsBackToClaudeProWhenMaxExhausted(t *testing.T) {
	// If max is at hard limit, should fall back to pro
	maxAcc := &Account{ID: "max1", Type: AccountTypeClaude, PlanType: "max", Usage: UsageSnapshot{
		PrimaryUsedPercent: 0.96, SecondaryUsedPercent: 0.5,
	}}
	proAcc := &Account{ID: "pro1", Type: AccountTypeClaude, PlanType: "pro", Usage: UsageSnapshot{
		PrimaryUsedPercent: 0.1, SecondaryUsedPercent: 0.1,
	}}
	p := newProviderPool([]*Account{proAcc, maxAcc}, false)

	got := p.candidate("", nil, AccountTypeClaude, "", "")
	if got == nil || got.ID != "pro1" {
		t.Fatalf("expected pro fallback when max exhausted, got %+v", got)
	}
}

func TestCandidateRequiredPlanOverridesBoundAffinity(t *testing.T) {
	plus := &Account{ID: "plus", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{PrimaryUsedPercent: 0.1}}
	pro := &Account{ID: "pro", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.2}}
	p := newProviderPool([]*Account{plus, pro}, false)
	p.bindAffinity("c1", "plus")

	got := p.candidate("c1", nil, AccountTypeCodex, "pro", "")
	if got == nil || got.ID != "pro" {
		t.Fatalf("expected bound plus to be bypassed for required plan, got %+v", got)
	}
	if binding, exists := p.convPin["c1"]; exists && binding.AccountID == "plus" {
		t.Fatalf("incompatible required-plan binding was not broken: %+v", binding)
	}
}

func TestCandidateKeepsBoundCodexPlus(t *testing.T) {
	plus := &Account{ID: "plus", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{PrimaryUsedPercent: 0.01, SecondaryUsedPercent: 0.01}}
	pro := &Account{ID: "pro", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.6, SecondaryUsedPercent: 0.6}}
	p := newProviderPool([]*Account{plus, pro}, false)
	p.bindAffinity("c1", "plus")

	got := p.candidate("c1", nil, AccountTypeCodex, "", "")
	if got == nil || got.ID != "plus" {
		t.Fatalf("expected healthy bound Plus affinity to remain eligible, got %+v", got)
	}
}

func TestCandidateAllCodexPlansCompeteByQuota(t *testing.T) {
	plus := &Account{ID: "plus", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{PrimaryUsedPercent: 0.01, SecondaryUsedPercent: 0.01}}
	pro := &Account{ID: "pro", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.8, SecondaryUsedPercent: 0.8}}
	p := newProviderPool([]*Account{plus, pro}, false)

	got := p.candidate("", nil, AccountTypeCodex, "", "")
	if got == nil || got.ID != "plus" {
		t.Fatalf("expected lower-pressure Plus to compete with Pro, got %+v", got)
	}
}

func TestCandidateWithCyberAccessOnlyReturnsMarkedAccounts(t *testing.T) {
	ordinary := &Account{ID: "ordinary", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.01, SecondaryUsedPercent: 0.01}}
	cyber := &Account{ID: "cyber", Type: AccountTypeCodex, PlanType: "pro", CyberAccess: true, Usage: UsageSnapshot{PrimaryUsedPercent: 0.2, SecondaryUsedPercent: 0.2}}
	p := newProviderPool([]*Account{ordinary, cyber}, false)

	got := p.candidateWithCyberAccess(nil, AccountTypeCodex, "", "")
	if got == nil || got.ID != "cyber" {
		t.Fatalf("expected cyber access account, got %+v", got)
	}
	if got := p.candidateWithCyberAccess(map[string]bool{"cyber": true}, AccountTypeCodex, "", ""); got != nil {
		t.Fatalf("expected no cyber access account after exclusion, got %+v", got)
	}
}

// Regression: when the only cyber-access account has an expired access
// token, candidateWithCyberAccess must still return it — the caller's
// dial path will lazy-refresh. Excluding expired accounts here causes
// cyber_policy errors to leak to the client when the only cyber account
// has been idle long enough to expire.
func TestCandidateWithCyberAccessReturnsExpiredAccount(t *testing.T) {
	expired := &Account{
		ID: "cyber-expired", Type: AccountTypeCodex, PlanType: "pro",
		CyberAccess: true,
		ExpiresAt:   time.Now().Add(-5 * time.Minute),
		Usage:       UsageSnapshot{PrimaryUsedPercent: 0.05, SecondaryUsedPercent: 0.05},
	}
	p := newProviderPool([]*Account{expired}, false)
	got := p.candidateWithCyberAccess(nil, AccountTypeCodex, "", "")
	if got == nil || got.ID != "cyber-expired" {
		t.Fatalf("expected expired cyber account to still be picked, got %+v", got)
	}
}

func TestCandidateDoesNotPileTrafficOntoHighWeeklyProLiteAccount(t *testing.T) {
	now := time.Now()
	proLite := &Account{
		ID:       "prolite",
		Type:     AccountTypeCodex,
		PlanType: "prolite",
		Usage: UsageSnapshot{
			SecondaryUsedPercent:   0.82,
			SecondaryWindowMinutes: codexWeeklyWindowMinutes,
			SecondaryResetAt:       now.Add(6 * 24 * time.Hour),
		},
	}
	pro := &Account{
		ID:       "pro",
		Type:     AccountTypeCodex,
		PlanType: "pro",
		Usage: UsageSnapshot{
			SecondaryUsedPercent:   0.02,
			SecondaryWindowMinutes: codexWeeklyWindowMinutes,
			SecondaryResetAt:       now.Add(6 * 24 * time.Hour),
		},
	}
	pool := newProviderPool([]*Account{proLite, pro}, false)

	got := pool.candidate("", nil, AccountTypeCodex, "", "")
	if got == nil {
		t.Fatal("expected a Codex candidate")
	}
	if got.ID != "pro" {
		t.Fatalf("candidate = %q, want lower-weekly-usage pro account", got.ID)
	}
}

func TestCandidateSkipsAccountWhenClientIPNotAllowed(t *testing.T) {
	restricted := &Account{ID: "restricted", Type: AccountTypeCodex, PlanType: "pro", AllowedSourceIPs: []string{"199.45.144.95"}, Usage: UsageSnapshot{PrimaryUsedPercent: 0.1}}
	fallback := &Account{ID: "fallback", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.2}}
	p := newProviderPool([]*Account{restricted, fallback}, false)

	got := p.candidate("", nil, AccountTypeCodex, "", "203.0.113.10")
	if got == nil || got.ID != "fallback" {
		t.Fatalf("expected unrestricted fallback, got %+v", got)
	}
}

func TestCandidateAllowsRestrictedAccountWhenClientIPMatches(t *testing.T) {
	restricted := &Account{ID: "restricted", Type: AccountTypeCodex, PlanType: "pro", AllowedSourceIPs: []string{"199.45.144.95"}, Usage: UsageSnapshot{PrimaryUsedPercent: 0.1}}
	fallback := &Account{ID: "fallback", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.4}}
	p := newProviderPool([]*Account{restricted, fallback}, false)

	got := p.candidate("", nil, AccountTypeCodex, "", "199.45.144.95")
	if got == nil || got.ID != "restricted" {
		t.Fatalf("expected restricted account for matching IP, got %+v", got)
	}
}

func TestMergeUsagePreservesExistingFields(t *testing.T) {
	prev := UsageSnapshot{
		PrimaryUsedPercent:   0.2,
		SecondaryUsedPercent: 0.3,
		PrimaryWindowMinutes: 300,
		Source:               "old",
		RetrievedAt:          time.Now(),
	}
	next := UsageSnapshot{
		PrimaryUsedPercent: 0.25,
		RetrievedAt:        time.Now().Add(1 * time.Minute),
		Source:             "body",
	}
	merged := mergeUsage(prev, next)
	if merged.SecondaryUsedPercent != 0.3 {
		t.Fatalf("expected secondary preserved when new absent, got %v", merged.SecondaryUsedPercent)
	}
	if merged.PrimaryWindowMinutes != 300 {
		t.Fatalf("expected window preserved, got %d", merged.PrimaryWindowMinutes)
	}
	if merged.PrimaryUsedPercent != 0.25 {
		t.Fatalf("expected primary updated, got %v", merged.PrimaryUsedPercent)
	}
	if merged.Source != "body" {
		t.Fatalf("expected source updated, got %s", merged.Source)
	}
}

func TestSaveAccountPreservesUnknownFields(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "auth.json")

	original := map[string]any{
		"tokens": map[string]any{
			"access_token":  "old-access",
			"refresh_token": "old-refresh",
			"id_token":      "old-id",
			"account_id":    "acct_123",
			"extra_token": map[string]any{
				"foo": 1,
			},
		},
		"allowed_ip":   "199.45.144.95",
		"cyber_access": true,
		"last_refresh": "2025-12-01T00:00:00Z",
		"extra_top":    []any{1, 2, 3},
		"meta": map[string]any{
			"x": "y",
		},
	}
	buf, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	acc := &Account{
		ID:               "a1",
		File:             path,
		AccessToken:      "new-access",
		RefreshToken:     "new-refresh",
		IDToken:          "new-id",
		AccountID:        "acct_123",
		AllowedSourceIPs: []string{"199.45.144.95"},
		CyberAccess:      true,
		LastRefresh:      time.Date(2025, 12, 17, 0, 0, 0, 0, time.UTC),
	}
	if err := saveAccount(acc); err != nil {
		t.Fatalf("saveAccount: %v", err)
	}

	afterRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var after map[string]any
	if err := json.Unmarshal(afterRaw, &after); err != nil {
		t.Fatalf("unmarshal after: %v", err)
	}

	// Top-level unknown fields preserved.
	if _, ok := after["extra_top"]; !ok {
		t.Fatalf("expected extra_top preserved")
	}
	if _, ok := after["meta"]; !ok {
		t.Fatalf("expected meta preserved")
	}
	if after["cyber_access"] != true {
		t.Fatalf("expected cyber_access preserved")
	}

	// Token fields updated, unknown token fields preserved.
	tokens, ok := after["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("expected tokens object")
	}
	if tokens["access_token"] != "new-access" {
		t.Fatalf("access_token=%v", tokens["access_token"])
	}
	if tokens["refresh_token"] != "new-refresh" {
		t.Fatalf("refresh_token=%v", tokens["refresh_token"])
	}
	if tokens["id_token"] != "new-id" {
		t.Fatalf("id_token=%v", tokens["id_token"])
	}
	if tokens["account_id"] != "acct_123" {
		t.Fatalf("account_id=%v", tokens["account_id"])
	}
	if _, ok := tokens["extra_token"]; !ok {
		t.Fatalf("expected tokens.extra_token preserved")
	}
}
