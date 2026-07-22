package main

import (
	"net/url"
	"sync"
	"testing"
)

func TestProviderRegistryReturnsDefensiveSnapshot(t *testing.T) {
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{})
	providers := registry.All()
	providers[0] = nil
	if registry.ForType(AccountTypeGemini) == nil {
		t.Fatal("caller mutated registry snapshot")
	}
}

func TestProviderRegistryRejectsDuplicateWithoutPublishing(t *testing.T) {
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{})
	before := registry.ForType(AccountTypeCodex)
	if err := registry.ReplaceExtras(&CodexProvider{}); err == nil {
		t.Fatal("duplicate provider accepted")
	}
	if registry.ForType(AccountTypeCodex) != before || len(registry.All()) != 3 {
		t.Fatal("failed replacement changed active snapshot")
	}
}

func TestProviderRegistryDeclarativeReplacementIsAtomic(t *testing.T) {
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{})
	first := validProviderSpec()
	if err := registry.ReplaceDeclarative([]ProviderSpec{first}); err != nil {
		t.Fatal(err)
	}
	active := registry.ForType(first.ID)
	if active == nil {
		t.Fatal("valid declarative provider not published")
	}
	invalid := first
	invalid.ID = "invalid"
	invalid.BaseURL = "/relative"
	if err := registry.ReplaceDeclarative([]ProviderSpec{invalid}); err == nil {
		t.Fatal("invalid declarative snapshot accepted")
	}
	if registry.ForType(first.ID) != active || registry.ForType(invalid.ID) != nil {
		t.Fatal("invalid reload changed active snapshot")
	}

	second := first
	second.BaseURL = "https://replacement.example.test/anthropic"
	if err := registry.ReplaceDeclarative([]ProviderSpec{second}); err != nil {
		t.Fatal(err)
	}
	got := registry.ForType(first.ID)
	if got == active || got.UpstreamURL("").String() != second.BaseURL {
		t.Fatalf("replacement not atomically published: %#v", got)
	}
}

func TestProviderRegistryConcurrentReadersSeeCompleteSnapshots(t *testing.T) {
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{})
	first := validProviderSpec()
	second := validProviderSpec()
	second.ID = "example-two"
	second.BaseURL = "https://two.example.test"
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 100 {
				providers := registry.All()
				if len(providers) < 3 || registry.ForType(AccountTypeCodex) == nil {
					t.Errorf("reader observed incomplete snapshot")
					return
				}
			}
		}()
	}
	for index := range 100 {
		spec := first
		if index%2 == 1 {
			spec = second
		}
		if err := registry.ReplaceDeclarative([]ProviderSpec{spec}); err != nil {
			t.Fatal(err)
		}
	}
	wait.Wait()
}

func TestDeepSeekAndZAIConstructorsBuildDeclarativeProviders(t *testing.T) {
	deepseekBase, _ := url.Parse("https://deepseek.example.test/anthropic")
	zaiBase, _ := url.Parse("https://zai.example.test/anthropic")
	for name, provider := range map[string]Provider{"deepseek": NewDeepSeekProvider(deepseekBase), "zai": NewZAIProvider(zaiBase)} {
		if _, ok := provider.(*DeclarativeProvider); !ok {
			t.Errorf("%s is not declarative: %T", name, provider)
		}
	}
}
