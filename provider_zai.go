package main

import "net/url"

// ZAIProvider is retained as a source-compatible name for the first
// declarative provider conversion.
type ZAIProvider = DeclarativeProvider

func NewZAIProvider(base *url.URL) *ZAIProvider {
	// Model data comes from the declarative spec (provider-specs/zai.json,
	// embedded as the builtin fallback) - the single data source.
	spec := mustBuiltinProviderSpec("zai")
	if base != nil {
		spec.BaseURL = base.String()
	}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		panic(err)
	}
	return provider
}

func isZAIModel(model string) bool {
	_, ok := NewZAIProvider(nil).MatchModel(model)
	return ok
}

func zaiCanonicalModel(model string) string {
	if found, ok := NewZAIProvider(nil).MatchModel(model); ok {
		return found.ID
	}
	return model
}
