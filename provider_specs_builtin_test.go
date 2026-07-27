package main

import (
	"testing"
)

func TestBuiltinKimiPlatformSpecOwnsRuntimeEndpointAndModels(t *testing.T) {
	spec, err := loadBuiltinProviderSpec("kimi-platform")
	if err != nil {
		t.Fatal(err)
	}
	provider := NewKimiPlatformProvider(nil)
	if provider.UpstreamURL("").String() != spec.BaseURL {
		t.Fatalf("runtime endpoint %q does not come from provider spec %q", provider.UpstreamURL(""), spec.BaseURL)
	}
	active := provider.Spec()
	if len(active.Models) != len(spec.Models) {
		t.Fatalf("runtime models=%d provider-spec models=%d", len(active.Models), len(spec.Models))
	}
	for index := range spec.Models {
		if active.Models[index].ID != spec.Models[index].ID {
			t.Fatalf("runtime model[%d]=%q provider-spec model=%q", index, active.Models[index].ID, spec.Models[index].ID)
		}
	}
}
