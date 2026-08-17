package main

import "net/url"

type MinimaxProvider = DeclarativeProvider

func NewMinimaxProvider(base *url.URL) *MinimaxProvider {
	// Model data comes from the declarative spec (provider-specs/minimax.json,
	// embedded as the builtin fallback) - the single data source. The code
	// var is retained only as a schema-shaped fallback when the JSON is absent.
	spec := mustBuiltinProviderSpec("minimax")
	if base != nil {
		spec.BaseURL = base.String()
	}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		panic(err)
	}
	return provider
}

func isMinimaxModel(model string) bool {
	_, ok := NewMinimaxProvider(nil).MatchModel(model)
	return ok
}

func minimaxCanonicalModel(model string) string {
	if found, ok := NewMinimaxProvider(nil).MatchModel(model); ok {
		return found.ID
	}
	return model
}
