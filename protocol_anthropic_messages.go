package main

// AnthropicMessagesEngine owns protocol-level usage normalization for
// Anthropic Messages responses and SSE events. Provider adapters delegate here
// and retain only provider-specific authentication, routing, and quota logic.
type AnthropicMessagesEngine struct{}

func (AnthropicMessagesEngine) ParseUsage(event map[string]any) *RequestUsage {
	return parseAnthropicUsage(event)
}

var anthropicMessagesEngine AnthropicMessagesEngine
