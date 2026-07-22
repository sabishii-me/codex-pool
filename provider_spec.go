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
	"time"
)

const (
	ProtocolAnthropicMessages = "anthropic-messages"
	ProtocolOpenAIChat        = "openai-chat"
	AuthBearer                = "bearer"
	AuthHeader                = "header"
	UsageAnthropicMessages    = "anthropic-messages"
	UsageOpenAIChat           = "openai-chat"
	UsageOpenAIChatKimi       = "openai-chat-kimi-billing"
	UsageResponses            = "openai-responses"
	QuotaNone                 = ""
	QuotaMinimax              = "minimax"
)

// ProviderSpec is the strict runtime schema for standard providers layered on
// a shared protocol engine. It deliberately describes data, not arbitrary
// behavior; unusual OAuth, signing, discovery, and transports remain plugins.
type ProviderSpec struct {
	ID               ProviderID       `json:"id"`
	Protocol         string           `json:"protocol"`
	BaseURL          string           `json:"base_url"`
	PlanType         string           `json:"plan_type"`
	CredentialField  string           `json:"credential_field"`
	Auth             ProviderAuthSpec `json:"auth"`
	UsageProfiles    []string         `json:"usage_profiles,omitempty"`
	QuotaProfile     string           `json:"quota_profile,omitempty"`
	ModelPrefix      string           `json:"model_prefix,omitempty"`
	StripModelPrefix bool             `json:"strip_model_prefix,omitempty"`
	Models           []ModelRouteSpec `json:"models"`
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
	spec          ProviderSpec
	baseURL       *url.URL
	usageProfiles []func(map[string]any) *RequestUsage
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
	switch spec.Protocol {
	case ProtocolAnthropicMessages, ProtocolOpenAIChat:
	default:
		return fmt.Errorf("provider %s: unsupported protocol %q", spec.ID, spec.Protocol)
	}
	profiles := spec.UsageProfiles
	if len(profiles) == 0 {
		profiles = []string{spec.Protocol}
	}
	for _, profile := range profiles {
		switch profile {
		case UsageAnthropicMessages, UsageOpenAIChat, UsageOpenAIChatKimi, UsageResponses:
		default:
			return fmt.Errorf("provider %s: unsupported usage profile %q", spec.ID, profile)
		}
	}
	if spec.QuotaProfile != QuotaNone && spec.QuotaProfile != QuotaMinimax {
		return fmt.Errorf("provider %s: unsupported quota profile %q", spec.ID, spec.QuotaProfile)
	}
	if spec.ModelPrefix != "" {
		if strings.TrimSpace(spec.ModelPrefix) != spec.ModelPrefix || strings.ContainsAny(spec.ModelPrefix, " \t\r\n") {
			return fmt.Errorf("provider %s: model_prefix must be non-whitespace", spec.ID)
		}
	} else if spec.StripModelPrefix {
		return fmt.Errorf("provider %s: strip_model_prefix requires model_prefix", spec.ID)
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
	profiles := spec.UsageProfiles
	if len(profiles) == 0 {
		profiles = []string{spec.Protocol}
	}
	for _, profile := range profiles {
		switch profile {
		case UsageAnthropicMessages:
			provider.usageProfiles = append(provider.usageProfiles, anthropicMessagesEngine.ParseUsage)
		case UsageOpenAIChat:
			provider.usageProfiles = append(provider.usageProfiles, openAIChatEngine.ParseUsage)
		case UsageOpenAIChatKimi:
			provider.usageProfiles = append(provider.usageProfiles, openAIChatLegacyKimiEngine.ParseUsage)
		case UsageResponses:
			provider.usageProfiles = append(provider.usageProfiles, openAIResponsesEngine.ParseUsage)
		}
	}
	return provider, nil
}

func cloneProviderSpec(spec ProviderSpec) ProviderSpec {
	clone := spec
	clone.UsageProfiles = append([]string(nil), spec.UsageProfiles...)
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
	if p != nil {
		for _, parse := range p.usageProfiles {
			if usage := parse(object); usage != nil {
				return usage
			}
		}
		if len(p.usageProfiles) > 0 {
			return nil
		}
	}
	// Preserve source compatibility for zero-value aliases of the first
	// Anthropic declarative providers. Runtime providers are always validated.
	return anthropicMessagesEngine.ParseUsage(object)
}
func (p *DeclarativeProvider) ParseUsageHeaders(connection *ProviderConnection, headers http.Header) {
	if p != nil && p.spec.QuotaProfile == QuotaMinimax {
		applyMinimaxRateLimits(connection, headers, time.Now())
	}
}
func (p *DeclarativeProvider) UpstreamURL(string) *url.URL      { return p.baseURL }
func (p *DeclarativeProvider) MatchesPath(string) bool          { return false }
func (p *DeclarativeProvider) NormalizePath(path string) string { return path }
func (p *DeclarativeProvider) DetectsSSE(path, contentType string) bool {
	return eventStreamDetector.Detect(path, contentType)
}

func modelRouteSpecsForProvider(providerID ProviderID) []ModelRouteSpec {
	models := modelsForProvider(providerID)
	specs := make([]ModelRouteSpec, 0, len(models))
	for _, model := range models {
		specs = append(specs, ModelRouteSpec{
			ID: model.ID, DisplayName: model.DisplayName, Description: model.Description,
			Aliases: append([]string(nil), model.Aliases...), ContextWindow: model.ContextWindow,
			MaxOutputTokens: model.MaxTokens, Reasoning: model.Reasoning, Input: append([]string(nil), model.Input...),
		})
	}
	return specs
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

func (p *DeclarativeProvider) TargetFormat() RequestFormat {
	if p != nil && p.spec.Protocol == ProtocolOpenAIChat {
		return FormatOpenAI
	}
	return FormatClaude
}

func (p *DeclarativeProvider) MatchModel(name string) (ModelRouteSpec, bool) {
	name = strings.TrimSpace(name)
	if p.spec.ModelPrefix != "" && strings.HasPrefix(name, p.spec.ModelPrefix) {
		canonical := name
		if p.spec.StripModelPrefix {
			canonical = strings.TrimPrefix(name, p.spec.ModelPrefix)
		}
		return ModelRouteSpec{ID: canonical}, true
	}
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
