package main

import "encoding/json"

// visionFallbackModel returns the vision-capable model that should handle an
// image-bearing request originally addressed to the given requested model, or
// "" when no explicit fallback is configured. The fallback must belong to the
// same declarative provider so the existing route/credential selection stays
// valid; cross-provider fallback is deliberately not inferred.
func visionFallbackModel(registry *ProviderRegistry, requestedModel string) string {
	if registry == nil || requestedModel == "" {
		return ""
	}
	declarative, ok := declarativeProviderForModel(registry, requestedModel)
	if !ok {
		return ""
	}
	route, ok := declarative.MatchModel(requestedModel)
	if !ok || route.VisionFallback == "" {
		return ""
	}
	// The fallback target must itself exist in the same provider spec; routing
	// through MatchModel again guarantees a real, resolvable model.
	if fallbackRoute, ok := declarative.MatchModel(route.VisionFallback); ok {
		return fallbackRoute.ID
	}
	return ""
}

// modelSupportsImage reports whether the declarative model accepts image input.
func modelSupportsImage(registry *ProviderRegistry, model string) bool {
	if registry == nil || model == "" {
		return false
	}
	declarative, ok := declarativeProviderForModel(registry, model)
	if !ok {
		return false
	}
	route, ok := declarative.MatchModel(model)
	if !ok {
		return false
	}
	for _, modality := range route.Input {
		if modality == "image" {
			return true
		}
	}
	return false
}

// declarativeProviderForModel returns the declarative provider that owns the
// given model name (or alias), if any.
func declarativeProviderForModel(registry *ProviderRegistry, model string) (*DeclarativeProvider, bool) {
	if registry == nil {
		return nil, false
	}
	for _, declarative := range registry.DeclarativeProviders() {
		if _, matched := declarative.MatchModel(model); matched {
			return declarative, true
		}
	}
	return nil, false
}

// requestBodyHasImage reports whether a request body carries image input across
// the three client protocols the gateway accepts (OpenAI chat, OpenAI
// responses, Anthropic messages). It only inspects the JSON shape; it does not
// validate URL/data payloads.
func requestBodyHasImage(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return false
	}
	if contentHasImage(obj["messages"]) {
		return true
	}
	if contentHasImage(obj["input"]) {
		return true
	}
	return false
}

// contentHasImage walks a messages/input value looking for image parts.
func contentHasImage(value any) bool {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if contentHasImage(item) {
				return true
			}
		}
	case map[string]any:
		typ, _ := v["type"].(string)
		switch typ {
		case "image_url", "input_image", "image":
			return true
		}
		if contentHasImage(v["content"]) {
			return true
		}
		if contentHasImage(v["image_url"]) {
			return true
		}
	}
	return false
}

// rewriteModelInBodyJSON is a test/observability-safe replacement for
// rewriteModelInBody that only rewrites the top-level "model" field.
func rewriteModelInBodyJSON(body []byte, newModel string) []byte {
	return rewriteModelInBody(body, newModel)
}
