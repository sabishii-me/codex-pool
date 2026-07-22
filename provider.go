package main

import (
	"context"
	"net/http"
	"net/url"
)

// Provider defines one upstream service and its protocol/credential behavior.
type Provider interface {
	Type() ProviderID

	// LoadAccount is the compatibility loader for a provider connection file.
	// New provider APIs should describe the result as ProviderConnection.
	LoadAccount(name, path string, data []byte) (*ProviderConnection, error)

	SetAuthHeaders(req *http.Request, connection *ProviderConnection)

	RefreshToken(ctx context.Context, connection *ProviderConnection, transport http.RoundTripper) error

	// ParseUsage extracts usage from an SSE event.
	// Returns nil if the event doesn't contain usage data.
	ParseUsage(obj map[string]any) *RequestUsage

	ParseUsageHeaders(connection *ProviderConnection, headers http.Header)

	// UpstreamURL returns the base URL for this provider.
	// Path is provided so providers can route different paths to different upstreams.
	UpstreamURL(path string) *url.URL

	MatchesPath(path string) bool

	NormalizePath(path string) string

	DetectsSSE(path string, contentType string) bool
}

// ProviderRegistry manages all provider implementations.
type ProviderRegistry struct {
	providers []Provider
	byType    map[ProviderID]Provider
}

// NewProviderRegistry creates a registry with all configured providers.
// Order matters for path matching: more specific patterns must come first.
// Claude (/v1/messages) must be checked before Codex (/v1/) to avoid false matches.
// Extra providers (e.g., Kimi, MiniMax) are model-routed and never win path matching.
func NewProviderRegistry(codex *CodexProvider, claude *ClaudeProvider, gemini *GeminiProvider, extra ...Provider) *ProviderRegistry {
	// Order: Gemini (unique paths), Claude (specific /v1/messages), Codex (broad /v1/)
	providers := []Provider{gemini, claude, codex}
	providers = append(providers, extra...)
	byType := make(map[ProviderID]Provider)
	for _, p := range providers {
		byType[p.Type()] = p
	}
	return &ProviderRegistry{
		providers: providers,
		byType:    byType,
	}
}

// ForType returns the provider for the given account type.
func (r *ProviderRegistry) ForType(providerID ProviderID) Provider {
	return r.byType[providerID]
}

// ForPath returns the provider that handles the given request path.
// Returns nil if no provider matches.
func (r *ProviderRegistry) ForPath(path string) Provider {
	for _, p := range r.providers {
		if p.MatchesPath(path) {
			return p
		}
	}
	return nil
}

// All returns all registered providers.
func (r *ProviderRegistry) All() []Provider {
	return r.providers
}
