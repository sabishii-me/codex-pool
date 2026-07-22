package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

const (
	ProtocolAnthropicMessages = "anthropic-messages"
	AuthBearer                = "bearer"
	AuthHeader                = "header"
)

// ProviderSpec is the strict runtime schema for standard providers layered on
// a shared protocol engine. It deliberately describes data, not arbitrary
// behavior; unusual OAuth, signing, discovery, and transports remain plugins.
type ProviderSpec struct {
	ID              ProviderID       `json:"id"`
	Protocol        string           `json:"protocol"`
	BaseURL         string           `json:"base_url"`
	PlanType        string           `json:"plan_type"`
	CredentialField string           `json:"credential_field"`
	Auth            ProviderAuthSpec `json:"auth"`
	Models          []ModelRouteSpec `json:"models"`
}

type ProviderAuthSpec struct {
	Type   string `json:"type"`
	Header string `json:"header,omitempty"`
	Prefix string `json:"prefix,omitempty"`
}

type ModelRouteSpec struct {
	ID              string   `json:"id"`
	DisplayName     string   `json:"display_name,omitempty"`
	Description     string   `json:"description,omitempty"`
	Aliases         []string `json:"aliases,omitempty"`
	ContextWindow   int      `json:"context_window,omitempty"`
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	Reasoning       bool     `json:"reasoning,omitempty"`
	Input           []string `json:"input,omitempty"`
}

// DeclarativeProvider is immutable after construction.
type DeclarativeProvider struct {
	spec    ProviderSpec
	baseURL *url.URL
	usage   func(map[string]any) *RequestUsage
}

func ParseProviderSpec(data []byte) (ProviderSpec, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var spec ProviderSpec
	if err := decoder.Decode(&spec); err != nil {
		return ProviderSpec{}, fmt.Errorf("decode provider spec: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return ProviderSpec{}, fmt.Errorf("decode provider spec: multiple JSON values")
		}
		return ProviderSpec{}, fmt.Errorf("decode provider spec: %w", err)
	}
	if err := ValidateProviderSpec(spec); err != nil {
		return ProviderSpec{}, err
	}
	return spec, nil
}

func ValidateProviderSpec(spec ProviderSpec) error {
	id := strings.TrimSpace(string(spec.ID))
	if id == "" {
		return fmt.Errorf("provider spec id is required")
	}
	if !isSafeProviderDirectoryName(id) {
		return fmt.Errorf("provider spec id %q must be a lowercase directory-safe slug", spec.ID)
	}
	if spec.Protocol != ProtocolAnthropicMessages {
		return fmt.Errorf("provider %s: unsupported protocol %q", spec.ID, spec.Protocol)
	}
	base, err := url.Parse(spec.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") {
		return fmt.Errorf("provider %s: invalid base_url %q", spec.ID, spec.BaseURL)
	}
	if strings.TrimSpace(spec.PlanType) == "" {
		return fmt.Errorf("provider %s: plan_type is required", spec.ID)
	}
	if strings.TrimSpace(spec.CredentialField) == "" {
		return fmt.Errorf("provider %s: credential_field is required", spec.ID)
	}
	switch spec.Auth.Type {
	case AuthBearer:
		if spec.Auth.Header != "" && !strings.EqualFold(spec.Auth.Header, "Authorization") {
			return fmt.Errorf("provider %s: bearer auth header must be Authorization", spec.ID)
		}
	case AuthHeader:
		if strings.TrimSpace(spec.Auth.Header) == "" {
			return fmt.Errorf("provider %s: header auth requires header", spec.ID)
		}
	default:
		return fmt.Errorf("provider %s: unsupported auth type %q", spec.ID, spec.Auth.Type)
	}
	seen := make(map[string]bool, len(spec.Models))
	for _, model := range spec.Models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			return fmt.Errorf("provider %s: model id is required", spec.ID)
		}
		key := strings.ToLower(id)
		if seen[key] {
			return fmt.Errorf("provider %s: duplicate model %q", spec.ID, id)
		}
		seen[key] = true
		if model.ContextWindow < 0 || model.MaxOutputTokens < 0 {
			return fmt.Errorf("provider %s model %s: token limits cannot be negative", spec.ID, id)
		}
	}
	return nil
}

func NewDeclarativeProvider(spec ProviderSpec) (*DeclarativeProvider, error) {
	if err := ValidateProviderSpec(spec); err != nil {
		return nil, err
	}
	base, _ := url.Parse(spec.BaseURL)
	provider := &DeclarativeProvider{spec: cloneProviderSpec(spec), baseURL: base}
	switch spec.Protocol {
	case ProtocolAnthropicMessages:
		provider.usage = anthropicMessagesEngine.ParseUsage
	}
	return provider, nil
}

func cloneProviderSpec(spec ProviderSpec) ProviderSpec {
	clone := spec
	clone.Models = append([]ModelRouteSpec(nil), spec.Models...)
	for index := range clone.Models {
		clone.Models[index].Aliases = append([]string(nil), spec.Models[index].Aliases...)
		clone.Models[index].Input = append([]string(nil), spec.Models[index].Input...)
	}
	return clone
}

func (p *DeclarativeProvider) Spec() ProviderSpec { return cloneProviderSpec(p.spec) }
func (p *DeclarativeProvider) Type() ProviderID   { return p.spec.ID }

func (p *DeclarativeProvider) LoadAccount(name, path string, data []byte) (*ProviderConnection, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	credential, _ := root[p.spec.CredentialField].(string)
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return nil, nil
	}
	return &ProviderConnection{
		Type: p.spec.ID, ID: strings.TrimSuffix(name, filepath.Ext(name)), File: path,
		AccessToken: credential, PlanType: p.spec.PlanType,
	}, nil
}

func (p *DeclarativeProvider) SetAuthHeaders(req *http.Request, connection *ProviderConnection) {
	switch p.spec.Auth.Type {
	case AuthBearer:
		prefix := p.spec.Auth.Prefix
		if prefix == "" {
			prefix = "Bearer "
		}
		req.Header.Set("Authorization", prefix+connection.AccessToken)
	case AuthHeader:
		req.Header.Set(p.spec.Auth.Header, p.spec.Auth.Prefix+connection.AccessToken)
	}
}

func (p *DeclarativeProvider) RefreshToken(context.Context, *ProviderConnection, http.RoundTripper) error {
	return nil
}
func (p *DeclarativeProvider) ParseUsage(object map[string]any) *RequestUsage {
	if p != nil && p.usage != nil {
		return p.usage(object)
	}
	// Preserve source compatibility for zero-value aliases of the first
	// Anthropic declarative providers. Runtime providers are always validated.
	return anthropicMessagesEngine.ParseUsage(object)
}
func (p *DeclarativeProvider) ParseUsageHeaders(*ProviderConnection, http.Header) {}
func (p *DeclarativeProvider) UpstreamURL(string) *url.URL                        { return p.baseURL }
func (p *DeclarativeProvider) MatchesPath(string) bool                            { return false }
func (p *DeclarativeProvider) NormalizePath(path string) string                   { return path }
func (p *DeclarativeProvider) DetectsSSE(path, contentType string) bool {
	return eventStreamDetector.Detect(path, contentType)
}

func isSafeProviderDirectoryName(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func (p *DeclarativeProvider) MatchModel(name string) (ModelRouteSpec, bool) {
	name = strings.TrimSpace(name)
	for _, model := range p.spec.Models {
		if strings.EqualFold(model.ID, name) {
			return model, true
		}
		for _, alias := range model.Aliases {
			if strings.EqualFold(alias, name) {
				return model, true
			}
		}
	}
	return ModelRouteSpec{}, false
}
