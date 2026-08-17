package main

import "testing"

func TestLoadGeneratedProviderSpecs(t *testing.T) {
	specs, err := LoadProviderSpecsDir("provider-specs")
	if err != nil {
		t.Fatalf("load provider-specs: %v", err)
	}
	if len(specs) == 0 {
		t.Fatal("no specs loaded")
	}
	for _, s := range specs {
		if len(s.Models) == 0 {
			t.Errorf("provider %s has no models", s.ID)
		}
	}
	t.Logf("loaded %d specs", len(specs))
}
