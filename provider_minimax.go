package main

import "net/url"

var minimaxProviderSpec = ProviderSpec{
	ID: AccountTypeMinimax, Protocol: ProtocolAnthropicMessages,
	BaseURL: "https://api.minimax.io/anthropic", PlanType: "minimax", CredentialField: "api_key",
	Auth: ProviderAuthSpec{Type: AuthBearer}, QuotaProfile: QuotaMinimax,
}

type MinimaxProvider = DeclarativeProvider

func NewMinimaxProvider(base *url.URL) *MinimaxProvider {
	spec := minimaxProviderSpec
	spec.Models = modelRouteSpecsForProvider(AccountTypeMinimax)
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
