package main

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderSpecsWatcherKeepsRegistryAndPoolOnInvalidReload(t *testing.T) {
	specsDir := t.TempDir()
	poolDir := t.TempDir()
	base, _ := url.Parse("https://initial.example.test/anthropic")
	provider := NewDeepSeekProvider(base)
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, provider)
	connection := &ProviderConnection{Type: AccountTypeDeepSeek, ID: "existing"}
	handler := &proxyHandler{cfg: &config{poolDir: poolDir}, registry: registry, pool: newProviderPool([]*ProviderConnection{connection}, false)}
	active := registry.ForType(AccountTypeDeepSeek)
	if err := os.WriteFile(filepath.Join(specsDir, "broken.json"), []byte(`{"id":"deepseek"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	watcher := &poolWatcher{providerSpecsDir: specsDir, handler: handler}
	watcher.reloadProviderSpecs()
	if registry.ForType(AccountTypeDeepSeek) != active {
		t.Fatal("invalid watcher reload changed provider snapshot")
	}
	if got := handler.pool.allAccounts(); len(got) != 1 || got[0] != connection {
		t.Fatal("invalid watcher reload changed connection pool")
	}
}

func TestProviderSpecsWatcherPublishesThenReloadsConnections(t *testing.T) {
	specsDir := t.TempDir()
	poolDir := t.TempDir()
	providerDir := filepath.Join(poolDir, "deepseek")
	if err := os.MkdirAll(providerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providerDir, "connection.json"), []byte(`{"api_key":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewDeepSeekProvider(nil))
	handler := &proxyHandler{cfg: &config{poolDir: poolDir}, registry: registry, pool: newProviderPool(nil, false)}
	spec := deepSeekProviderSpec
	spec.BaseURL = "https://replacement.example.test/anthropic"
	writeProviderSpecTestFile(t, specsDir, "deepseek.json", spec)
	watcher := &poolWatcher{providerSpecsDir: specsDir, handler: handler}
	watcher.reloadProviderSpecs()
	if got := registry.ForType(AccountTypeDeepSeek).UpstreamURL("").String(); got != spec.BaseURL {
		t.Fatalf("active base=%q", got)
	}
	connections := handler.pool.allAccounts()
	if len(connections) != 1 || connections[0].ID != "connection" || connections[0].AccessToken != "secret" {
		t.Fatalf("connections=%#v", connections)
	}
}
