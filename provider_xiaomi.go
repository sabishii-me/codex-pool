package main

import "net/url"

type XiaomiProvider = DeclarativeProvider

func NewXiaomiProvider(base *url.URL) *XiaomiProvider {
	// Model data comes from the declarative spec (provider-specs/xiaomi.json,
	// embedded as the builtin fallback) - the single data source.
	spec := mustBuiltinProviderSpec("xiaomi")
	if base != nil {
		spec.BaseURL = base.String()
	}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		panic(err)
	}
	return provider
}

func isXiaomiModel(model string) bool {
	_, ok := NewXiaomiProvider(nil).MatchModel(model)
	return ok
}

func xiaomiCanonicalModel(model string) string {
	if found, ok := NewXiaomiProvider(nil).MatchModel(model); ok {
		return found.ID
	}
	return model
}
