package main

import "net/url"

var zaiProviderSpec = ProviderSpec{
	ID: AccountTypeZAI, Protocol: ProtocolAnthropicMessages,
	BaseURL: "https://api.z.ai/api/anthropic", PlanType: "zai", CredentialField: "api_key",
	Auth: ProviderAuthSpec{Type: AuthHeader, Header: "X-Api-Key"},
	Models: []ModelRouteSpec{
		{ID: "glm-5.2", ContextWindow: 1000000, MaxOutputTokens: 65536},
	},
}

// ZAIProvider is retained as a source-compatible name for the first
// declarative provider conversion.
type ZAIProvider = DeclarativeProvider

func NewZAIProvider(base *url.URL) *ZAIProvider {
	spec := zaiProviderSpec
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
