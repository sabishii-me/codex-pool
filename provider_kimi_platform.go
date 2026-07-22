package main

import (
	"net/url"
	"strings"
)

const kimiPlatformModelPrefix = "kimi-platform/"

var kimiPlatformProviderSpec = ProviderSpec{
	ID: AccountTypeKimiPlatform, Protocol: ProtocolAnthropicMessages,
	BaseURL: "https://api.moonshot.ai/anthropic", PlanType: "kimi-platform", CredentialField: "api_key",
	Auth:          ProviderAuthSpec{Type: AuthBearer},
	UsageProfiles: []string{UsageAnthropicMessages, UsageOpenAIChatKimi},
	ModelPrefix:   kimiPlatformModelPrefix, StripModelPrefix: true,
}

type KimiPlatformProvider = DeclarativeProvider

func NewKimiPlatformProvider(base *url.URL) *KimiPlatformProvider {
	spec := kimiPlatformProviderSpec
	spec.Models = modelRouteSpecsForProvider(AccountTypeKimiPlatform)
	if base != nil {
		spec.BaseURL = base.String()
	}
	provider, err := NewDeclarativeProvider(spec)
	if err != nil {
		panic(err)
	}
	return provider
}

func isKimiPlatformModel(model string) bool {
	_, ok := NewKimiPlatformProvider(nil).MatchModel(model)
	return ok
}

func kimiPlatformCanonicalModel(model string) string {
	if found, ok := NewKimiPlatformProvider(nil).MatchModel(model); ok {
		return found.ID
	}
	return strings.TrimSpace(model)
}
