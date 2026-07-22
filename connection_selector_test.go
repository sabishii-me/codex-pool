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
