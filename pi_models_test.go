package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGeneratePiModelsJSON(t *testing.T) {
	t.Setenv("POOL_NAME", "test-pool")

	data, err := generatePiModelsJSON(
		"https://pool.example.com/",
		"codex-token",
		"sk-ant-oat01-pool-test",
		nil, // nil pool -> every model's provider is treated as available
		nil, // nil registry -> only plugin providers (codex/grok/kimi) contribute
	)
	if err != nil {
		t.Fatalf("generatePiModelsJSON error: %v", err)
	}

	var cfg piModelsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal pi models json: %v", err)
	}

	if len(cfg.Providers) != 1 {
		t.Fatalf("expected exactly one provider, got %d", len(cfg.Providers))
	}
	prov, ok := cfg.Providers["test-pool"]
	if !ok {
		t.Fatalf("provider %q missing; got providers %v", "test-pool", cfg.Providers)
	}
	if prov.BaseURL != "https://pool.example.com" {
		t.Fatalf("pool baseUrl = %q, want https://pool.example.com (no /backend-api suffix)", prov.BaseURL)
	}
	if prov.API != "anthropic-messages" {
		t.Fatalf("pool api = %q", prov.API)
	}
	if prov.APIKey != "sk-ant-oat01-pool-test" {
		t.Fatalf("pool apiKey = %q", prov.APIKey)
	}

	byID := map[string]piModelConfig{}
	for _, m := range prov.Models {
		byID[m.ID] = m
	}
	for _, id := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex-spark"} {
		m, ok := byID[id]
		if !ok {
			t.Fatalf("model %q missing from pool provider", id)
		}
		if len(m.Input) == 0 {
			t.Fatalf("model %q has no input kinds", id)
		}
	}
	if m := byID["gpt-5.6-sol"]; m.ThinkingLevelMap["xhigh"] != "xhigh" || m.ThinkingLevelMap["max"] != "max" {
		t.Fatalf("gpt-5.6-sol thinking levels = %#v, want xhigh+max", m.ThinkingLevelMap)
	}
}

func TestGeneratedClientConfigsIncludeDiscoveredAntigravityModels(t *testing.T) {
	t.Setenv("POOL_NAME", "test-pool")
	antigravityModels.Reset()
	t.Cleanup(antigravityModels.Reset)
	antigravityModels.ReplaceAccount("antigravity-test", AntigravityAccountSnapshot{
		FetchedAt: time.Now(),
		Models: map[string]AntigravityModelInfo{
			"gemini-live": {ID: "gemini-live", DisplayName: "Gemini Live", MaxTokens: 1000000, MaxOutputTokens: 65536, SupportsThinking: true},
		},
	})
	piJSON, err := generatePiModelsJSON("https://pool.example.com", "codex-token", "claude-token", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var piConfig piModelsConfig
	if err := json.Unmarshal(piJSON, &piConfig); err != nil {
		t.Fatal(err)
	}
	prov := piConfig.Providers["test-pool"]
	found := false
	for _, m := range prov.Models {
		if m.ID == "antigravity/gemini-live" {
			found = true
		}
	}
	if !found {
		t.Fatalf("antigravity/gemini-live missing from pool provider models")
	}
}

func TestAvailablePiModelsFiltersUnavailableProviders(t *testing.T) {
	t.Setenv("POOL_NAME", "test-pool")
	// Pool only has a codex account. Providers without any account must not
	// contribute models to the Pi list (e.g. deepseek, zai would be unavailable).
	pool := &ProviderPool{accounts: []*ProviderConnection{
		{ID: "codex-1", Type: AccountTypeCodex},
	}}
	models := availablePiModels(pool, nil, nil)
	byID := map[string]bool{}
	for _, m := range models {
		byID[m.ID] = true
	}
	if !byID["gpt-5.6-sol"] {
		t.Fatalf("gpt-5.6-sol missing; codex has an account so it should be present")
	}
	if byID["deepseek-v4-flash"] {
		t.Fatalf("deepseek-v4-flash should be filtered out (no deepseek account)")
	}
	if byID["glm-5.2"] {
		t.Fatalf("glm-5.2 should be filtered out (no zai account)")
	}
}

func TestIsKimiModelHandlesPiBuiltInIDs(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"kimi", "kimi-for-coding", "k2p5", "kimi-k2-thinking"} {
		if !isKimiModel(model) {
			t.Fatalf("expected %q to route to kimi", model)
		}
	}
}

func TestMinimaxCanonicalModelHandlesPiBuiltInIDs(t *testing.T) {
	t.Parallel()

	// The declarative spec (provider-specs/minimax.json) is the single data
	// source: model IDs use the official/upstream lowercase IDs.
	tests := map[string]string{
		"minimax":     "minimax-m3",
		"minimax-m3":  "minimax-m3",
		"MiniMax-M3":  "minimax-m3",
		"minimax-m2.7": "minimax-m2.7",
	}

	for input, want := range tests {
		if got := minimaxCanonicalModel(input); got != want {
			t.Fatalf("minimaxCanonicalModel(%q) = %q, want %q", input, got, want)
		}
		if !isMinimaxModel(input) {
			t.Fatalf("expected %q to route to minimax", input)
		}
	}
}

func TestGrokCanonicalModelHandlesCodeAliases(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"grok-4.5":                     "grok-4.5",
		"grok-4.5-build":               "grok-4.5",
		"grok-build":                   "grok-build",
		"grok-composer-2.5-fast":       "grok-composer-2.5-fast",
		"grok-composer":                "grok-composer-2.5-fast",
		"grok-code-fast":               "grok-composer-2.5-fast",
		"grok-4.3":                     "grok-4.3",
		"grok-4.20-0309-reasoning":     "grok-4.20-0309-reasoning",
		"grok-4.20-0309-non-reasoning": "grok-4.20-0309-non-reasoning",
		"grok-4.20-multi-agent-0309":   "grok-4.20-multi-agent-0309",
	}

	for input, want := range tests {
		if got := grokCanonicalModel(input); got != want {
			t.Fatalf("grokCanonicalModel(%q) = %q, want %q", input, got, want)
		}
		if !isGrokModel(input) {
			t.Fatalf("expected %q to route to grok", input)
		}
	}
}
