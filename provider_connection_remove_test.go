package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveProviderConnectionDeletesCredentialAndReloadsPool(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "google-ai-image")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	credential := filepath.Join(directory, "remove-me.json")
	if err := os.WriteFile(credential, []byte(`{"type":"google-ai-image","api_key":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := NewGoogleAIImageProvider(nil)
	registry := NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, provider)
	accounts, err := loadPool(root, registry)
	if err != nil {
		t.Fatal(err)
	}
	h := &proxyHandler{cfg: &config{poolDir: root}, pool: newProviderPool(accounts), registry: registry}
	response := httptest.NewRecorder()
	h.removeProviderConnection(response, "remove-me")
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(credential); !os.IsNotExist(err) {
		t.Fatalf("credential still exists: %v", err)
	}
	if h.pool.countByType(AccountTypeGoogleAIImage) != 0 {
		t.Fatal("removed connection remains in pool")
	}
}

func TestRemoveProviderConnectionRejectsCredentialOutsidePool(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	account := &ProviderConnection{Type: AccountTypeGoogleAIImage, ID: "outside", File: outside}
	h := &proxyHandler{cfg: &config{poolDir: root}, pool: newProviderPool([]*ProviderConnection{account})}
	response := httptest.NewRecorder()
	h.removeProviderConnection(response, "outside")
	if response.Code != 409 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside credential changed: %v", err)
	}
	account.mu.Lock()
	disabled := account.Disabled
	account.mu.Unlock()
	if disabled {
		t.Fatal("failed removal changed connection state")
	}
}
