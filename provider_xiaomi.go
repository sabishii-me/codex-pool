package main

import "net/url"

var xiaomiProviderSpec = ProviderSpec{
	ID: AccountTypeXiaomi, Protocol: ProtocolAnthropicMessages,
	BaseURL: "https://token-plan-sgp.xiaomimimo.com/anthropic", PlanType: "xiaomi", CredentialField: "api_key",
	Auth:          ProviderAuthSpec{Type: AuthBearer},
	UsageProfiles: []string{UsageAnthropicMessages, UsageResponses},
}

type XiaomiProvider = DeclarativeProvider

func NewXiaomiProvider(base *url.URL) *XiaomiProvider {
	spec := xiaomiProviderSpec
	spec.Models = modelRouteSpecsForProvider(AccountTypeXiaomi)
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
