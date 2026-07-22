package main

import (
	"testing"
	"time"
)

func TestConnectionSelectorDelegatesGeneralPinnedAndExactSelection(t *testing.T) {
	first := &ProviderConnection{Type: AccountTypeClaude, ID: "first", PlanType: "max"}
	second := &ProviderConnection{Type: AccountTypeClaude, ID: "second", PlanType: "max"}
	pool := newProviderPool([]*ProviderConnection{first, second}, false)
	pool.pin("conversation", second.ID)
	selector := NewConnectionSelector(pool)

	pinned := selector.Select(ConnectionSelection{ProviderID: AccountTypeClaude, ConversationID: "conversation"})
	if pinned != second {
		t.Fatalf("pinned=%v, want second", pinned)
	}
	exact := selector.Select(ConnectionSelection{Mode: SelectExactID, ProviderID: AccountTypeClaude, ConnectionID: first.ID})
	if exact != first {
		t.Fatalf("exact=%v, want first", exact)
	}
	if got := selector.Select(ConnectionSelection{Mode: SelectExactID, ProviderID: AccountTypeCodex, ConnectionID: first.ID}); got != nil {
		t.Fatalf("cross-provider exact selection=%v", got)
	}
}

func TestConnectionSelectorDelegatesCyberAndCooldownSelection(t *testing.T) {
	ordinary := &ProviderConnection{Type: AccountTypeCodex, ID: "ordinary", PlanType: "pro"}
	cyber := &ProviderConnection{Type: AccountTypeCodex, ID: "cyber", PlanType: "pro", CyberAccess: true}
	cooling := &ProviderConnection{Type: AccountTypeClaude, ID: "cooling", RateLimitUntil: time.Now().Add(time.Minute)}
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{ordinary, cyber, cooling}, false))

	got := selector.Select(ConnectionSelection{Mode: SelectCyberAccess, ProviderID: AccountTypeCodex})
	if got != cyber {
		t.Fatalf("cyber=%v, want cyber", got)
	}
	wait := selector.NearestCooldown(AccountTypeClaude, nil)
	if wait <= 0 || wait > time.Minute {
		t.Fatalf("cooldown=%v", wait)
	}
}

func TestConnectionSelectorDelegatesAntigravityModelCapability(t *testing.T) {
	antigravityModels.Reset()
	t.Cleanup(antigravityModels.Reset)
	connection := &ProviderConnection{Type: AccountTypeAntigravity, ID: "ag", ModelRateLimits: map[string]time.Time{}}
	antigravityModels.ReplaceAccount(connection.ID, AntigravityAccountSnapshot{Models: map[string]AntigravityModelInfo{"gemini-test": {ID: "gemini-test"}}})
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{connection}, false))
	got := selector.Select(ConnectionSelection{Mode: SelectModelCapability, ProviderID: AccountTypeAntigravity, Model: "gemini-test"})
	if got != connection {
		t.Fatalf("model-capability selection=%v", got)
	}
	if got := selector.Select(ConnectionSelection{Mode: SelectModelCapability, ProviderID: AccountTypeAntigravity, Model: "missing"}); got != nil {
		t.Fatalf("unsupported model selected=%v", got)
	}
}

func TestConnectionSelectorWeightsCompetitiveCodexWithoutStarvation(t *testing.T) {
	connections := []*ProviderConnection{
		{ID: "cyber-a", Type: AccountTypeCodex, PlanType: "pro", CyberAccess: true, Usage: UsageSnapshot{SecondaryUsedPercent: 0.1}},
		{ID: "cyber-b", Type: AccountTypeCodex, PlanType: "pro", CyberAccess: true, Usage: UsageSnapshot{SecondaryUsedPercent: 0.1}},
		{ID: "ordinary-a", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{SecondaryUsedPercent: 0.1}},
		{ID: "ordinary-b", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{SecondaryUsedPercent: 0.1}},
	}
	selector := NewConnectionSelector(newProviderPool(connections, false))
	counts := map[string]int{}
	for range 60 {
		selected := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex})
		if selected == nil {
			t.Fatal("expected Codex connection")
		}
		counts[selected.ID]++
	}
	for _, id := range []string{"cyber-a", "cyber-b"} {
		if counts[id] != 20 {
			t.Fatalf("%s selections=%d, want 20; all=%v", id, counts[id], counts)
		}
	}
	for _, id := range []string{"ordinary-a", "ordinary-b"} {
		if counts[id] != 10 {
			t.Fatalf("%s selections=%d, want 10; all=%v", id, counts[id], counts)
		}
	}
}

func TestConnectionSelectorExcludesNoncompetitiveCodexFromFairRotation(t *testing.T) {
	healthy := &ProviderConnection{ID: "healthy", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{SecondaryUsedPercent: 0.1}}
	drainedCyber := &ProviderConnection{ID: "drained-cyber", Type: AccountTypeCodex, PlanType: "pro", CyberAccess: true, Usage: UsageSnapshot{SecondaryUsedPercent: 0.5}}
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{drainedCyber, healthy}, false))
	for i := 0; i < 12; i++ {
		if selected := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex}); selected != healthy {
			t.Fatalf("selection %d=%v, want quota-healthy ordinary connection", i, selected)
		}
	}
}

func TestConnectionSelectorCyberRetryRemainsCyberOnly(t *testing.T) {
	ordinary := &ProviderConnection{ID: "ordinary", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{SecondaryUsedPercent: 0}}
	cyber := &ProviderConnection{ID: "cyber", Type: AccountTypeCodex, PlanType: "pro", CyberAccess: true, Usage: UsageSnapshot{SecondaryUsedPercent: 0.5}}
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{ordinary, cyber}, false))
	for i := 0; i < 4; i++ {
		if selected := selector.Select(ConnectionSelection{Mode: SelectCyberAccess, ProviderID: AccountTypeCodex}); selected != cyber {
			t.Fatalf("cyber retry selected=%v", selected)
		}
	}
}

func TestConnectionSelectorOwnsImageCapabilityAndFanout(t *testing.T) {
	unsupported := &ProviderConnection{Type: AccountTypeCodex, ID: "unsupported", ImageGenerationSupport: -1}
	first := &ProviderConnection{Type: AccountTypeCodex, ID: "first", ImageGenerationSupport: 1}
	second := &ProviderConnection{Type: AccountTypeCodex, ID: "second", ImageGenerationSupport: 1}
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{unsupported, first, second}, false))

	general := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex, RequireImages: true})
	if general == unsupported || general == nil {
		t.Fatalf("image-capable general selection=%v", general)
	}
	fanout := selector.Select(ConnectionSelection{Mode: SelectImageFanout, ProviderID: AccountTypeCodex, FanoutIndex: 1, RequireImages: true})
	if fanout != second {
		t.Fatalf("fanout selection=%v, want second", fanout)
	}
}
