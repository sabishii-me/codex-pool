package main

import (
	"crypto/sha256"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPrivateAffinityKeyValidation(t *testing.T) {
	valid := affinityRoutingKey("secret", "user", AccountTypeCodex, "model", "openai_responses", ClientAffinitySignal{Kind: AffinityClientSession, Value: "session"})
	if valid == "" {
		t.Fatal("valid affinity fixture did not derive a key")
	}
	if !isPrivateAffinityKey(valid) {
		t.Fatalf("valid key rejected: %q", valid)
	}
	for _, invalid := range []string{"", "aff:v1:conversation", "aff:v1:" + strings.Repeat("g", sha256.Size*2), valid + "00"} {
		if isPrivateAffinityKey(invalid) {
			t.Fatalf("invalid private key accepted: %q", invalid)
		}
	}
}

func TestRoutingContextMatchRequiresCompleteTypedContext(t *testing.T) {
	key := affinityRoutingKey("secret", "user", AccountTypeCodex, "model", "openai_responses", ClientAffinitySignal{Kind: AffinityClientSession, Value: "session"})
	valid := RequestRoutingContext{Provider: AccountTypeCodex, Protocol: "openai_responses", CanonicalModel: "model", SoftAffinity: ClientAffinitySignal{Kind: AffinityClientSession}, AffinityKey: key}
	if !routingContextMatchesSelection(valid, AccountTypeCodex) {
		t.Fatal("complete typed routing context was rejected")
	}
	invalid := valid
	invalid.CanonicalModel = ""
	if routingContextMatchesSelection(invalid, AccountTypeCodex) {
		t.Fatal("routing context without canonical model was accepted")
	}
	invalid = valid
	invalid.AffinityKey = "aff:v1:conversation"
	if routingContextMatchesSelection(invalid, AccountTypeCodex) {
		t.Fatal("routing context with malformed private key was accepted")
	}
	invalid = valid
	invalid.SoftAffinity.Kind = ""
	if routingContextMatchesSelection(invalid, AccountTypeCodex) {
		t.Fatal("routing context without affinity kind was accepted")
	}
}

func TestConnectionSelectorDelegatesTypedAffinityAndExactSelection(t *testing.T) {
	first := &ProviderConnection{Type: AccountTypeCodex, ID: "first", PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0.1, SecondaryWindowMinutes: 10080}}
	second := &ProviderConnection{Type: AccountTypeCodex, ID: "second", PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0.8, SecondaryWindowMinutes: 10080}}
	pool := newProviderPool([]*ProviderConnection{first, second}, false)
	privateKey := affinityRoutingKey("test-secret", "test-user", AccountTypeCodex, "test-model", "openai_responses", ClientAffinitySignal{Kind: AffinityClientSession, Value: "conversation"})
	if privateKey == "" {
		t.Fatal("typed affinity fixture did not derive a key")
	}
	context := RequestRoutingContext{Provider: AccountTypeCodex, Protocol: "openai_responses", CanonicalModel: "test-model", SoftAffinity: ClientAffinitySignal{Kind: AffinityClientSession}, AffinityKey: privateKey}
	pool.bindAffinity(privateKey, second.ID)
	selector := NewConnectionSelector(pool)

	bound := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex, RoutingContext: context})
	if bound != second {
		t.Fatalf("bound=%v, want second", bound)
	}
	// Typed context is authoritative when legacy compatibility identity is also present.
	pool.bindAffinity("legacy", first.ID)
	if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex, RoutingContext: context, ConversationID: "legacy"}); got != second {
		t.Fatalf("typed routing context lost precedence: %v", got)
	}
	if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex, RoutingContext: RequestRoutingContext{AffinityKey: "conversation"}, ConversationID: "conversation"}); got != first {
		t.Fatalf("untyped routing key reused a binding: %v", got)
	}
	if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex, RoutingContext: RequestRoutingContext{Provider: AccountTypeCodex, Protocol: "openai_responses", AffinityKey: "raw-conversation"}, ConversationID: privateKey}); got != first {
		t.Fatalf("non-HMAC routing context reused a binding: %v", got)
	}
	if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeClaude, RoutingContext: RequestRoutingContext{Provider: AccountTypeCodex, Protocol: "openai_responses", CanonicalModel: "test-model", SoftAffinity: ClientAffinitySignal{Kind: AffinityClientSession}, AffinityKey: privateKey}, ConversationID: "conversation"}); got != nil {
		t.Fatalf("cross-provider routing context selected a Codex connection: %v", got)
	}
	exact := selector.Select(ConnectionSelection{Mode: SelectExactID, ProviderID: AccountTypeCodex, ConnectionID: first.ID})
	if exact != first {
		t.Fatalf("exact=%v, want first", exact)
	}
	if got := selector.Select(ConnectionSelection{Mode: SelectExactID, ProviderID: AccountTypeClaude, ConnectionID: first.ID}); got != nil {
		t.Fatalf("cross-provider exact selection=%v", got)
	}
}

func TestConnectionSelectorRollbackDisablesTypedAffinityWithoutLegacyFallback(t *testing.T) {
	first := &ProviderConnection{ID: "a", Type: AccountTypeCodex, PlanType: "plus"}
	bound := &ProviderConnection{ID: "z", Type: AccountTypeCodex, PlanType: "plus"}
	pool := newProviderPool([]*ProviderConnection{first, bound}, false)
	privateKey := "aff:v1:" + strings.Repeat("a", 64)
	pool.bindAffinity(privateKey, bound.ID)
	selector := NewConnectionSelector(pool)
	selector.typedAffinityEnabled = false

	context := RequestRoutingContext{Provider: AccountTypeCodex, Protocol: "openai_responses", CanonicalModel: "model", SoftAffinity: ClientAffinitySignal{Kind: AffinityPromptCacheKey}, AffinityKey: privateKey}
	if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex, RoutingContext: context, ConversationID: privateKey}); got != first {
		t.Fatalf("disabled affinity selected %v, want ordinary candidate %v", got, first)
	}
}

func TestCodexBalancingRollbackUsesLegacyUnweightedSelection(t *testing.T) {
	ordinary := &ProviderConnection{ID: "a", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0.1, secondarySet: true}}
	cyber := &ProviderConnection{ID: "b", Type: AccountTypeCodex, PlanType: "plus", CyberAccess: true, Usage: UsageSnapshot{SecondaryUsedPercent: 0.1, secondarySet: true}}
	pool := newProviderPool([]*ProviderConnection{ordinary, cyber}, false)
	pool.setRoutingFeatures(false)
	selector := NewConnectionSelector(pool)
	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		got := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex})
		if got == nil {
			t.Fatal("nil selection")
		}
		counts[got.ID]++
	}
	if counts[ordinary.ID] != 3 || counts[cyber.ID] != 3 {
		t.Fatalf("rollback selection=%v, want legacy unweighted rotation", counts)
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

func TestSelectQuotaCompetitiveConnectionIgnoresNilCandidates(t *testing.T) {
	known := &ProviderConnection{ID: "known"}
	got := selectQuotaCompetitiveConnection([]weightedConnectionCandidate{
		{connection: nil, score: 100, telemetrySet: true},
		{connection: known, score: 1, telemetrySet: true},
	}, 0)
	if got != known {
		t.Fatalf("nil candidate distorted selection: %v", got)
	}
}

func TestConnectionSelectorRequiredPlanOverridesTypedPlusAffinity(t *testing.T) {
	plus := &ProviderConnection{ID: "plus", Type: AccountTypeCodex, PlanType: "plus"}
	pro := &ProviderConnection{ID: "pro", Type: AccountTypeCodex, PlanType: "pro"}
	pool := newProviderPool([]*ProviderConnection{plus, pro}, false)
	context := buildRequestRoutingContext("/v1/responses", nil, http.Header{"Session_id": []string{"plus-session"}}, "test-user", AccountTypeCodex, "test-model", "test-secret")
	if context.AffinityKey == "" || !routingContextMatchesSelection(context, AccountTypeCodex) {
		t.Fatal("test context did not derive private affinity")
	}
	pool.bindAffinity(context.AffinityKey, plus.ID)
	selector := NewConnectionSelector(pool)
	if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex, RequiredPlan: "pro", RoutingContext: context}); got != pro {
		t.Fatalf("required plan did not override soft affinity: %v", got)
	}
	if binding, exists := pool.convPin[context.AffinityKey]; exists && binding.AccountID == plus.ID {
		t.Fatalf("required-plan mismatch retained incompatible affinity: %+v", binding)
	}
}

func TestConnectionSelectorUsesBothQuotaWindows(t *testing.T) {
	primaryConstrained := &ProviderConnection{ID: "primary-constrained", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{PrimaryUsedPercent: 0.9, SecondaryUsedPercent: 0.1, PrimaryWindowMinutes: 300, SecondaryWindowMinutes: 10080}}
	balanced := &ProviderConnection{ID: "balanced", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{PrimaryUsedPercent: 0.2, SecondaryUsedPercent: 0.2, PrimaryWindowMinutes: 300, SecondaryWindowMinutes: 10080}}
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{primaryConstrained, balanced}, false))
	if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex}); got != balanced {
		t.Fatalf("primary pressure was ignored: %v", got)
	}
}

func TestConnectionSelectorExploresWhenAllTelemetryUnknown(t *testing.T) {
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{
		{ID: "unknown-a", Type: AccountTypeCodex, PlanType: "plus"},
		{ID: "unknown-b", Type: AccountTypeCodex, PlanType: "pro"},
	}, false))
	seen := map[string]int{}
	for range 10 {
		selected := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex})
		if selected == nil {
			t.Fatal("all-unknown pool returned no connection")
		}
		seen[selected.ID]++
	}
	if seen["unknown-a"] == 0 || seen["unknown-b"] == 0 {
		t.Fatalf("all-unknown pool was not explored: %v", seen)
	}
}

func TestConnectionSelectorDoesNotTreatUnknownTelemetryAsFreeCapacity(t *testing.T) {
	known := &ProviderConnection{ID: "known", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0.2, SecondaryWindowMinutes: 10080, secondarySet: true}}
	unknown := &ProviderConnection{ID: "unknown", Type: AccountTypeCodex, PlanType: "plus"}
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{unknown, known}, false))
	for range 10 {
		if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex}); got != known {
			t.Fatalf("unknown telemetry selected as free capacity: %v", got)
		}
	}
}

func TestConnectionSelectorUsesUnderusedPlusCapacityBeforeDrainingPro(t *testing.T) {
	connections := []*ProviderConnection{
		{ID: "plus-a", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0, secondarySet: true}},
		{ID: "plus-b", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0, secondarySet: true}},
		{ID: "plus-c", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0, secondarySet: true}},
		{ID: "plus-d", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0, secondarySet: true}},
		{ID: "plus-e", Type: AccountTypeCodex, PlanType: "plus", Usage: UsageSnapshot{SecondaryUsedPercent: 0, secondarySet: true}},
		{ID: "pro", Type: AccountTypeCodex, PlanType: "pro", Usage: UsageSnapshot{SecondaryUsedPercent: 0.32, secondarySet: true}},
	}
	selector := NewConnectionSelector(newProviderPool(connections, false))
	seen := map[string]int{}
	for range 25 {
		selected := selector.Select(ConnectionSelection{ProviderID: AccountTypeCodex})
		if selected == nil {
			t.Fatal("expected Codex connection")
		}
		seen[selected.ID]++
	}
	if seen["pro"] != 0 {
		t.Fatalf("32%% Pro should not drain while five Plus connections are unused: %v", seen)
	}
	for _, id := range []string{"plus-a", "plus-b", "plus-c", "plus-d", "plus-e"} {
		if seen[id] != 5 {
			t.Fatalf("%s selections=%d, want deterministic 5; all=%v", id, seen[id], seen)
		}
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
