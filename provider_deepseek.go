package main

import "net/url"

var deepSeekProviderSpec = ProviderSpec{
	ID: AccountTypeDeepSeek, Protocol: ProtocolAnthropicMessages,
	BaseURL: "https://api.deepseek.com/anthropic", PlanType: "deepseek", CredentialField: "api_key",
	Auth: ProviderAuthSpec{Type: AuthBearer},
	Models: []ModelRouteSpec{
		{ID: "deepseek-v4-flash", Aliases: []string{"deepseek-flash"}, ContextWindow: 128000, MaxOutputTokens: 32768},
		{ID: "deepseek-v4-pro", Aliases: []string{"deepseek-pro", "deepseek"}, ContextWindow: 128000, MaxOutputTokens: 65536},
	},
}

// DeepSeekProvider is retained as a source-compatible name for the first
// declarative provider conversion.
type DeepSeekProvider = DeclarativeProvider

func NewDeepSeekProvider(base *url.URL) *DeepSeekProvider {
	spec := deepSeekProviderSpec
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
