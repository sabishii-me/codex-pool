package main

import "strings"

// ModelBodyPolicy declares how a resolved route may transform the request.
// Large streamed bodies are safe only with ModelBodyRewriteNative.
type ModelBodyPolicy int

const (
	ModelBodyRewriteNative ModelBodyPolicy = iota
	ModelBodySanitizeGrok
	ModelBodyCustomAntigravity
)

// ResolvedModelRoute is the single routing decision shared by normal,
// streamed-body, and future WebSocket model-aware request paths.
type ResolvedModelRoute struct {
	Provider       Provider
	RequestedModel string
	CanonicalModel string
	BodyPolicy     ModelBodyPolicy
	RewriteModel   bool
}

func (route ResolvedModelRoute) RequiresWholeBody() bool {
	return route.BodyPolicy != ModelBodyRewriteNative
}

func (route ResolvedModelRoute) RewriteBody(body []byte) []byte {
	switch route.BodyPolicy {
	case ModelBodySanitizeGrok:
		return rewriteAndSanitizeGrokRequestBody(body, route.CanonicalModel)
	case ModelBodyRewriteNative:
		if !route.RewriteModel && route.CanonicalModel == route.RequestedModel {
			return nil
		}
		return rewriteModelInBody(body, route.CanonicalModel)
	default:
		return nil
	}
}

// ModelRouteRegistry resolves model ownership independently from body size and
// transport. Provider-specific execution remains explicit through BodyPolicy.
type ModelRouteRegistry struct {
	providers *ProviderRegistry
}

func NewModelRouteRegistry(providers *ProviderRegistry) *ModelRouteRegistry {
	return &ModelRouteRegistry{providers: providers}
}

func (registry *ModelRouteRegistry) Resolve(path, model string) (ResolvedModelRoute, bool) {
	model = strings.TrimSpace(model)
	if registry == nil || registry.providers == nil || model == "" {
		return ResolvedModelRoute{}, false
	}
	if provider, canonical, ok := registry.providers.MatchDeclarativeModel(model); ok {
		return ResolvedModelRoute{Provider: provider, RequestedModel: model, CanonicalModel: canonical, BodyPolicy: ModelBodyRewriteNative, RewriteModel: true}, true
	}
	if shouldRouteAntigravityModel(model) {
		if provider := registry.providers.ForType(AccountTypeAntigravity); provider != nil {
			return ResolvedModelRoute{Provider: provider, RequestedModel: model, CanonicalModel: antigravityCanonicalModel(model), BodyPolicy: ModelBodyCustomAntigravity}, true
		}
	}
	if isKimiModel(model) {
		if provider := registry.providers.ForType(AccountTypeKimi); provider != nil {
			return ResolvedModelRoute{Provider: provider, RequestedModel: model, CanonicalModel: model, BodyPolicy: ModelBodyRewriteNative}, true
		}
	}
	if isGrokModel(model) {
		if provider := registry.providers.ForType(AccountTypeGrok); provider != nil {
			return ResolvedModelRoute{Provider: provider, RequestedModel: model, CanonicalModel: grokCanonicalModel(model), BodyPolicy: ModelBodySanitizeGrok, RewriteModel: true}, true
		}
	}
	if isOpenAIModel(model) {
		if provider := registry.providers.ForType(AccountTypeCodex); provider != nil {
			return ResolvedModelRoute{Provider: provider, RequestedModel: model, CanonicalModel: model, BodyPolicy: ModelBodyRewriteNative}, true
		}
	}
	if isClaudeModel(model) && !isCodexToClaudeModelOverridePath(path) {
		if provider := registry.providers.ForType(AccountTypeClaude); provider != nil {
			return ResolvedModelRoute{Provider: provider, RequestedModel: model, CanonicalModel: claudeCanonicalModel(model), BodyPolicy: ModelBodyRewriteNative, RewriteModel: true}, true
		}
	}
	return ResolvedModelRoute{}, false
}
