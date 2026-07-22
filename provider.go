package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
)

// ProviderIdentity identifies an upstream provider independently of its other
// capabilities.
type ProviderIdentity interface {
	Type() ProviderID
}

// ProviderCredentialLoader decodes persisted credentials into a connection.
type ProviderCredentialLoader interface {
	ProviderIdentity
	LoadAccount(name, path string, data []byte) (*ProviderConnection, error)
}

// ProviderAuthenticator applies connection credentials to an upstream request.
type ProviderAuthenticator interface {
	SetAuthHeaders(req *http.Request, connection *ProviderConnection)
}

// ProviderRefresher renews an existing provider connection.
type ProviderRefresher interface {
	RefreshToken(ctx context.Context, connection *ProviderConnection, transport http.RoundTripper) error
}

// ProviderUsageParser normalizes provider response payloads.
type ProviderUsageParser interface {
	ParseUsage(obj map[string]any) *RequestUsage
}

// ProviderUsageHeaderParser updates quota state from response headers.
type ProviderUsageHeaderParser interface {
	ParseUsageHeaders(connection *ProviderConnection, headers http.Header)
}

// ProviderRouteTarget owns path matching and upstream URL normalization.
type ProviderRouteTarget interface {
	ProviderIdentity
	UpstreamURL(path string) *url.URL
	MatchesPath(path string) bool
	NormalizePath(path string) string
}

// ProviderStreamDetector classifies provider responses without owning usage
// normalization or transport execution.
type ProviderStreamDetector interface {
	DetectsSSE(path string, contentType string) bool
}

// Provider is the compatibility composition used where one request crosses
// credential, routing, authentication, usage, and streaming boundaries. New
// isolated components should accept the smallest capability above.
type Provider interface {
	ProviderCredentialLoader
	ProviderAuthenticator
	ProviderRefresher
	ProviderUsageParser
	ProviderUsageHeaderParser
	ProviderRouteTarget
	ProviderStreamDetector
}

type providerRegistrySnapshot struct {
	providers []Provider
	byType    map[ProviderID]Provider
}

// ProviderRegistry publishes immutable snapshots. Readers never observe a
// partially validated declarative reload.
type ProviderRegistry struct {
	mu       sync.Mutex
	core     []Provider
	baseline []Provider
	snapshot atomic.Pointer[providerRegistrySnapshot]
}

func NewProviderRegistry(codex *CodexProvider, claude *ClaudeProvider, gemini *GeminiProvider, extra ...Provider) *ProviderRegistry {
	core := []Provider{gemini, claude, codex}
	providers := append(core, extra...)
	registry := &ProviderRegistry{core: append([]Provider(nil), core...), baseline: append([]Provider(nil), providers...)}
	registry.publish(providers)
	return registry
}

func buildProviderRegistrySnapshot(providers []Provider) (*providerRegistrySnapshot, error) {
	snapshot := &providerRegistrySnapshot{
		providers: append([]Provider(nil), providers...),
		byType:    make(map[ProviderID]Provider, len(providers)),
	}
	for _, provider := range providers {
		if provider == nil {
			return nil, fmt.Errorf("provider registry contains nil provider")
		}
		id := provider.Type()
		if id == "" {
			return nil, fmt.Errorf("provider registry contains empty provider id")
		}
		if _, exists := snapshot.byType[id]; exists {
			return nil, fmt.Errorf("provider registry contains duplicate provider %q", id)
		}
		snapshot.byType[id] = provider
	}
	return snapshot, nil
}

func (registry *ProviderRegistry) publish(providers []Provider) {
	snapshot, err := buildProviderRegistrySnapshot(providers)
	if err != nil {
		panic(err)
	}
	registry.snapshot.Store(snapshot)
}

// ReplaceExtras atomically replaces every provider after the core
// Gemini/Claude/Codex path-routing set. It is useful for tests and code-backed
// plugin assembly.
func (registry *ProviderRegistry) ReplaceExtras(extras ...Provider) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	providers := append(append([]Provider(nil), registry.core...), extras...)
	snapshot, err := buildProviderRegistrySnapshot(providers)
	if err != nil {
		return err
	}
	registry.baseline = append([]Provider(nil), providers...)
	registry.snapshot.Store(snapshot)
	return nil
}

// AddProviders atomically appends code-backed plugins to the active snapshot.
func (registry *ProviderRegistry) AddProviders(providers ...Provider) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	combined := append([]Provider(nil), registry.baseline...)
	combined = append(combined, providers...)
	snapshot, err := buildProviderRegistrySnapshot(combined)
	if err != nil {
		return err
	}
	registry.baseline = append([]Provider(nil), combined...)
	registry.snapshot.Store(snapshot)
	return nil
}

// ReplaceDeclarative atomically replaces declarative providers while retaining
// all current code-backed providers. Every spec is validated and constructed
// before the new snapshot is published.
func (registry *ProviderRegistry) ReplaceDeclarative(specs []ProviderSpec) error {
	declarative := make([]Provider, 0, len(specs))
	seen := make(map[ProviderID]bool, len(specs))
	for _, spec := range specs {
		if seen[spec.ID] {
			return fmt.Errorf("duplicate declarative provider %q", spec.ID)
		}
		provider, err := NewDeclarativeProvider(spec)
		if err != nil {
			return err
		}
		seen[spec.ID] = true
		declarative = append(declarative, provider)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	providers := make([]Provider, 0, len(registry.baseline)+len(declarative))
	for _, provider := range registry.baseline {
		if !seen[provider.Type()] {
			providers = append(providers, provider)
		}
	}
	providers = append(providers, declarative...)
	snapshot, err := buildProviderRegistrySnapshot(providers)
	if err != nil {
		return err
	}
	registry.snapshot.Store(snapshot)
	return nil
}

func (registry *ProviderRegistry) ForType(providerID ProviderID) Provider {
	if snapshot := registry.snapshot.Load(); snapshot != nil {
		return snapshot.byType[providerID]
	}
	return nil
}

func (registry *ProviderRegistry) ForPath(path string) Provider {
	if snapshot := registry.snapshot.Load(); snapshot != nil {
		for _, provider := range snapshot.providers {
			if provider.MatchesPath(path) {
				return provider
			}
		}
	}
	return nil
}

func (registry *ProviderRegistry) MatchDeclarativeModel(model string) (Provider, string, bool) {
	if snapshot := registry.snapshot.Load(); snapshot != nil {
		for _, provider := range snapshot.providers {
			declarative, ok := provider.(*DeclarativeProvider)
			if !ok {
				continue
			}
			if route, matched := declarative.MatchModel(model); matched {
				return provider, route.ID, true
			}
		}
	}
	return nil, model, false
}

func (registry *ProviderRegistry) DeclarativeProviders() []*DeclarativeProvider {
	var providers []*DeclarativeProvider
	if snapshot := registry.snapshot.Load(); snapshot != nil {
		for _, provider := range snapshot.providers {
			if declarative, ok := provider.(*DeclarativeProvider); ok {
				providers = append(providers, declarative)
			}
		}
	}
	return providers
}

func (registry *ProviderRegistry) All() []Provider {
	if snapshot := registry.snapshot.Load(); snapshot != nil {
		return append([]Provider(nil), snapshot.providers...)
	}
	return nil
}
