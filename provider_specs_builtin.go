package main

import (
	"embed"
	"fmt"
)

//go:embed provider-specs.builtin/*.json
var builtinProviderSpecFiles embed.FS

func loadBuiltinProviderSpec(name string) (ProviderSpec, error) {
	data, err := builtinProviderSpecFiles.ReadFile("provider-specs.builtin/" + name + ".json")
	if err != nil {
		return ProviderSpec{}, fmt.Errorf("read built-in provider spec %s: %w", name, err)
	}
	spec, err := ParseProviderSpec(data)
	if err != nil {
		return ProviderSpec{}, fmt.Errorf("parse built-in provider spec %s: %w", name, err)
	}
	return spec, nil
}

func mustBuiltinProviderSpec(name string) ProviderSpec {
	spec, err := loadBuiltinProviderSpec(name)
	if err != nil {
		panic(err)
	}
	return spec
}
