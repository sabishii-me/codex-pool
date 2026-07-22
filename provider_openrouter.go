package main

import (
	"net/url"
	"strings"
)

const openrouterModelPrefix = "openrouter/"

var openRouterProviderSpec = ProviderSpec{
	ID: AccountTypeOpenRouter, Protocol: ProtocolAnthropicMessages,
	BaseURL: "https://openrouter.ai/api", PlanType: "openrouter", CredentialField: "api_key",
	Auth:        ProviderAuthSpec{Type: AuthBearer},
	ModelPrefix: openrouterModelPrefix, StripModelPrefix: true,
}

type OpenRouterProvider = DeclarativeProvider

func NewOpenRouterProvider(base *url.URL) *OpenRouterProvider {
	spec := openRouterProviderSpec
	spec.Models = modelRouteSpecsForProvider(AccountTypeOpenRouter)
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
