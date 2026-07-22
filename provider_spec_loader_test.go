package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeProviderSpecTestFile(t *testing.T, dir, name string, spec ProviderSpec) {
	t.Helper()
	data, err := jsonMarshalIndent(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func jsonMarshalIndent(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

func TestLoadProviderSpecsDirIsDeterministicAndStrict(t *testing.T) {
	dir := t.TempDir()
	first := validProviderSpec()
	first.ID = "z-provider"
	second := validProviderSpec()
	second.ID = "a-provider"
	writeProviderSpecTestFile(t, dir, "z.json", first)
	writeProviderSpecTestFile(t, dir, "a.json", second)
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	specs, err := LoadProviderSpecsDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].ID != "a-provider" || specs[1].ID != "z-provider" {
		t.Fatalf("spec order=%#v", specs)
	}
	if err := os.WriteFile(filepath.Join(dir, "invalid.json"), []byte(`{"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProviderSpecsDir(dir); err == nil {
		t.Fatal("invalid file did not reject directory snapshot")
	}
}

func TestReloadProviderSpecsKeepsPreviousSnapshotOnFailure(t *testing.T) {
	dir := t.TempDir()
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{})
	spec := validProviderSpec()
	writeProviderSpecTestFile(t, dir, "provider.json", spec)
	if err := ReloadProviderSpecs(registry, dir); err != nil {
		t.Fatal(err)
	}
	active := registry.ForType(spec.ID)
	if active == nil {
		t.Fatal("provider not loaded")
	}
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte(`{"id":"broken"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReloadProviderSpecs(registry, dir); err == nil {
		t.Fatal("invalid reload accepted")
	}
	if registry.ForType(spec.ID) != active {
		t.Fatal("failed filesystem reload changed active snapshot")
	}
}

func TestLoadProviderSpecsDirRejectsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	spec := validProviderSpec()
	writeProviderSpecTestFile(t, dir, "one.json", spec)
	writeProviderSpecTestFile(t, dir, "two.json", spec)
	if _, err := LoadProviderSpecsDir(dir); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
}
