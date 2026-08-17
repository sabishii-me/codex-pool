package main

import (
	"net/url"
	"strings"
)

const nvidiaModelPrefix = "nvidia/"

type NvidiaProvider = DeclarativeProvider

func NewNvidiaProvider(base *url.URL) *NvidiaProvider {
	// Model data comes from the declarative spec (provider-specs/nvidia.json,
	// embedded as the builtin fallback) - the single data source.
	spec := mustBuiltinProviderSpec("nvidia")
	if base != nil {
		spec.BaseURL = base.String()
	}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		panic(err)
	}
	return provider
}

func isNvidiaModel(model string) bool {
	_, ok := NewNvidiaProvider(nil).MatchModel(model)
	return ok
}

func nvidiaCanonicalModel(model string) string {
	if found, ok := NewNvidiaProvider(nil).MatchModel(model); ok {
		return found.ID
	}
	return strings.TrimSpace(model)
}
