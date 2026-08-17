package main

import (
	"net/url"
	"strings"
)

const openrouterModelPrefix = "openrouter/"

type OpenRouterProvider = DeclarativeProvider

func NewOpenRouterProvider(base *url.URL) *OpenRouterProvider {
	// Model data comes from the declarative spec (provider-specs/openrouter.json,
	// embedded as the builtin fallback) - the single data source.
	spec := mustBuiltinProviderSpec("openrouter")
	if base != nil {
		spec.BaseURL = base.String()
	}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		panic(err)
	}
	return provider
}

func isOpenRouterModel(model string) bool {
	_, ok := NewOpenRouterProvider(nil).MatchModel(model)
	return ok
}

func openrouterCanonicalModel(model string) string {
	if found, ok := NewOpenRouterProvider(nil).MatchModel(model); ok {
		return found.ID
	}
	return strings.TrimSpace(model)
}
