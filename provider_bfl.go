package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BFLProvider represents Black Forest Labs' native media API. It is not an
// LLM protocol adapter; request execution is owned by native_image_bfl.go.
type BFLProvider struct{ base *url.URL }

func NewBFLProvider(base *url.URL) *BFLProvider { return &BFLProvider{base: base} }
func (p *BFLProvider) Type() AccountType        { return AccountTypeBFL }

func (p *BFLProvider) LoadAccount(name, path string, data []byte) (*ProviderConnection, error) {
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
		CreditsBalance     *float64          `json:"credits_balance"`
		CreditsRetrievedAt time.Time         `json:"credits_retrieved_at"`
	}
	if err := json.Unmarshal(data, &credential); err != nil {
		return nil, err
	}
	if credential.Type != "" && credential.Type != string(AccountTypeBFL) {
		return nil, nil
	}
	if strings.TrimSpace(credential.APIKey) == "" {
		return nil, nil
	}
	plan := credential.PlanType
	if plan == "" {
		plan = "credits"
	}
	connection := &ProviderConnection{Type: AccountTypeBFL, ID: strings.TrimSuffix(name, ".json"), File: path, AccessToken: credential.APIKey, Disabled: credential.Disabled, Dead: credential.Dead, PlanType: plan, AddedAt: credential.AddedAt, Identity: ConnectionIdentity{DisplayName: credential.DisplayName, ExternalSubject: credential.ExternalSubject, Attributes: credential.IdentityAttributes}}
	if credential.CreditsBalance != nil {
		connection.Usage.CreditsBalance, connection.Usage.HasCredits, connection.Usage.RetrievedAt, connection.Usage.Source = *credential.CreditsBalance, true, credential.CreditsRetrievedAt, "bfl_credits"
		connection.Usage.creditsSet = true
	}
	return connection, nil
}
func (p *BFLProvider) SetAuthHeaders(req *http.Request, connection *ProviderConnection) {
	req.Header.Set("x-key", connection.AccessToken)
	req.Header.Set("accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
}
func (p *BFLProvider) FetchCredits(ctx context.Context, transport http.RoundTripper, connection *ProviderConnection) (float64, error) {
	if p == nil || p.base == nil || transport == nil || connection == nil {
		return 0, errors.New("BFL credits are unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.base.String(), "/")+"/v1/credits", nil)
	if err != nil {
		return 0, err
	}
	p.SetAuthHeaders(req, connection)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return 0, fmt.Errorf("BFL credits returned status %d", resp.StatusCode)
	}
	var result struct {
		Credits float64 `json:"credits"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || result.Credits < 0 {
		return 0, errors.New("BFL credits response was invalid")
	}
	return result.Credits, nil
}

func (p *BFLProvider) RefreshToken(context.Context, *ProviderConnection, http.RoundTripper) error {
	return errors.New("BFL API keys do not refresh")
}
func (p *BFLProvider) ParseUsage(map[string]any) *RequestUsage            { return nil }
func (p *BFLProvider) ParseUsageHeaders(*ProviderConnection, http.Header) {}
func (p *BFLProvider) UpstreamURL(string) *url.URL                        { return p.base }
func (p *BFLProvider) MatchesPath(string) bool                            { return false }
func (p *BFLProvider) NormalizePath(path string) string                   { return path }
func (p *BFLProvider) DetectsSSE(string, string) bool                     { return false }
