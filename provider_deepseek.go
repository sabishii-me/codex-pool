package main

import "net/url"

// DeepSeekProvider is retained as a source-compatible name for the first
// declarative provider conversion.
type DeepSeekProvider = DeclarativeProvider

func NewDeepSeekProvider(base *url.URL) *DeepSeekProvider {
	// Model data comes from the declarative spec (provider-specs/deepseek.json,
	// embedded as the builtin fallback) - the single data source.
	spec := mustBuiltinProviderSpec("deepseek")
	if base != nil {
		spec.BaseURL = base.String()
	}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		panic(err)
	}
	return provider
}

func isDeepSeekModel(model string) bool {
	_, ok := NewDeepSeekProvider(nil).MatchModel(model)
	return ok
}

func deepseekCanonicalModel(model string) string {
	if found, ok := NewDeepSeekProvider(nil).MatchModel(model); ok {
		return found.ID
	}
	return model
}
