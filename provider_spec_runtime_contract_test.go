package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeDeclarativeProviderLoadsRoutesAndPublishesModels(t *testing.T) {
	poolDir := t.TempDir()
	spec := validProviderSpec()
	spec.ID = "runtime-provider"
	spec.Models[0] = ModelRouteSpec{
		ID: "runtime-model", DisplayName: "Runtime Model", Description: "Loaded without recompiling",
		Aliases: []string{"runtime"}, ContextWindow: 64000, MaxOutputTokens: 8000, Reasoning: true, Input: []string{"text"},
	}
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{})
	if err := registry.ReplaceDeclarative([]ProviderSpec{spec}); err != nil {
		t.Fatal(err)
	}
	providerDir := filepath.Join(poolDir, string(spec.ID))
	if err := os.MkdirAll(providerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providerDir, "runtime.json"), []byte(`{"api_key":"runtime-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	connections, err := loadPool(poolDir, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].Type != spec.ID || connections[0].AccessToken != "runtime-secret" {
		t.Fatalf("connections=%#v", connections)
	}
	handler := &proxyHandler{registry: registry, aliases: newModelAliases(nil)}
	provider, base, rewritten := handler.modelRouteOverride("/v1/messages", "runtime", []byte(`{"model":"runtime"}`))
	if provider == nil || provider.Type() != spec.ID || base.String() != spec.BaseURL || !bytes.Equal(rewritten, []byte(`{"model":"runtime-model"}`)) {
		t.Fatalf("route provider=%v base=%v body=%s", provider, base, rewritten)
	}

	pool := newProviderPool(connections, false)
	recorder := httptest.NewRecorder()
	servePoolModelsWithRegistry(recorder, pool, registry)
	if recorder.Code != http.StatusOK {
		t.Fatalf("catalog status=%d", recorder.Code)
	}
	var catalog struct {
		Models []poolModelDescriptor `json:"models"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	var found *poolModelDescriptor
	for index := range catalog.Models {
		if catalog.Models[index].ID == "runtime-model" {
			found = &catalog.Models[index]
			break
		}
	}
	if found == nil || found.Provider != string(spec.ID) || found.ContextWindow != 64000 || found.MaxOutputTokens != 8000 || !found.AvailableNow || found.AvailableAccounts != 1 {
		t.Fatalf("runtime catalog model=%#v", found)
	}
}

func TestProviderSpecRejectsUnsafeDirectoryID(t *testing.T) {
	for _, id := range []ProviderID{"../escape", "Uppercase", "with space", "under_score"} {
		spec := validProviderSpec()
		spec.ID = id
		if err := ValidateProviderSpec(spec); err == nil {
			t.Errorf("unsafe id %q accepted", id)
		}
	}
}
