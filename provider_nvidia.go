package main

import (
	"net/url"
	"strings"
)

const nvidiaModelPrefix = "nvidia/"

var nvidiaProviderSpec = ProviderSpec{
	ID: AccountTypeNvidia, Protocol: ProtocolOpenAIChat,
	BaseURL: "https://integrate.api.nvidia.com/v1", PlanType: "nvidia", CredentialField: "api_key",
	Auth:        ProviderAuthSpec{Type: AuthBearer},
	ModelPrefix: nvidiaModelPrefix, StripModelPrefix: true,
}

type NvidiaProvider = DeclarativeProvider

func NewNvidiaProvider(base *url.URL) *NvidiaProvider {
	spec := nvidiaProviderSpec
	spec.Models = modelRouteSpecsForProvider(AccountTypeNvidia)
	if base != nil {
		spec.BaseURL = base.String()
	}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		panic(err)
	}
	return provider
}

func isNvidiaModel(model string) bool {
	_, ok := NewNvidiaProvider(nil).MatchModel(model)
	return ok
}

func nvidiaCanonicalModel(model string) string {
	if found, ok := NewNvidiaProvider(nil).MatchModel(model); ok {
		return found.ID
	}
	return strings.TrimSpace(model)
}
