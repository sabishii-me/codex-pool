package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GoogleAIImageProvider is the Google AI Studio native image API-key pool.
// It is deliberately separate from Gemini LLM OAuth and Antigravity OAuth.
type GoogleAIImageProvider struct{ base *url.URL }

func NewGoogleAIImageProvider(base *url.URL) *GoogleAIImageProvider {
	return &GoogleAIImageProvider{base: base}
}
func (p *GoogleAIImageProvider) Type() AccountType { return AccountTypeGoogleAIImage }

func (p *GoogleAIImageProvider) LoadAccount(name, path string, data []byte) (*ProviderConnection, error) {
	var credential struct {
		Type               string            `json:"type"`
		APIKey             string            `json:"api_key"`
		Disabled           bool              `json:"disabled"`
		Dead               bool              `json:"dead"`
		PlanType           string            `json:"plan_type"`
		AddedAt            time.Time         `json:"added_at"`
		DisplayName        string            `json:"display_name"`
		ExternalSubject    string            `json:"external_subject"`
		IdentityAttributes map[string]string `json:"identity_attributes"`
	}
	if err := json.Unmarshal(data, &credential); err != nil {
		return nil, err
	}
	if credential.Type != "" && credential.Type != string(AccountTypeGoogleAIImage) {
		return nil, nil
	}
	if strings.TrimSpace(credential.APIKey) == "" {
		return nil, nil
	}
	plan := credential.PlanType
	if plan == "" {
		plan = "ai-studio"
	}
	return &ProviderConnection{Type: AccountTypeGoogleAIImage, ID: strings.TrimSuffix(name, ".json"), File: path, AccessToken: credential.APIKey, Disabled: credential.Disabled, Dead: credential.Dead, PlanType: plan, AddedAt: credential.AddedAt, Identity: ConnectionIdentity{DisplayName: credential.DisplayName, ExternalSubject: credential.ExternalSubject, Attributes: credential.IdentityAttributes}}, nil
}
func (p *GoogleAIImageProvider) SetAuthHeaders(req *http.Request, connection *ProviderConnection) {
	req.Header.Set("x-goog-api-key", connection.AccessToken)
	req.Header.Set("accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
}
func (p *GoogleAIImageProvider) RefreshToken(context.Context, *ProviderConnection, http.RoundTripper) error {
	return errors.New("Google AI Studio API keys do not refresh")
}
func (p *GoogleAIImageProvider) ParseUsage(map[string]any) *RequestUsage            { return nil }
func (p *GoogleAIImageProvider) ParseUsageHeaders(*ProviderConnection, http.Header) {}
func (p *GoogleAIImageProvider) UpstreamURL(string) *url.URL                        { return p.base }
func (p *GoogleAIImageProvider) MatchesPath(string) bool                            { return false }
func (p *GoogleAIImageProvider) NormalizePath(path string) string                   { return path }
func (p *GoogleAIImageProvider) DetectsSSE(string, string) bool                     { return false }
