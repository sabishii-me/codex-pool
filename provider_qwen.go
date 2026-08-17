package main

import "net/url"

type QwenProvider = DeclarativeProvider

func NewQwenProvider(base *url.URL) *QwenProvider {
	// Model data comes from the declarative spec (provider-specs/qwen.json,
	// embedded as the builtin fallback) - the single data source.
	spec := mustBuiltinProviderSpec("qwen")
	if base != nil {
		spec.BaseURL = base.String()
	}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		panic(err)
	}
	return provider
}

func isQwenModel(model string) bool {
	_, ok := NewQwenProvider(nil).MatchModel(model)
	return ok
}

func qwenCanonicalModel(model string) string {
	if found, ok := NewQwenProvider(nil).MatchModel(model); ok {
		return found.ID
	}
	return model
}
