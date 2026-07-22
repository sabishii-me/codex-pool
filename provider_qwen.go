package main

import "net/url"

var qwenProviderSpec = ProviderSpec{
	ID: AccountTypeQwen, Protocol: ProtocolAnthropicMessages,
	BaseURL: "https://coding-intl.dashscope.aliyuncs.com/apps/anthropic", PlanType: "qwen", CredentialField: "api_key",
	Auth: ProviderAuthSpec{Type: AuthBearer},
}

type QwenProvider = DeclarativeProvider

func NewQwenProvider(base *url.URL) *QwenProvider {
	spec := qwenProviderSpec
	spec.Models = modelRouteSpecsForProvider(AccountTypeQwen)
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
