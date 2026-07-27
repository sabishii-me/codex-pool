package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProviderConnectionsV2UsesCanonicalDomainContract(t *testing.T) {
	connection := &ProviderConnection{
		Type: AccountTypeCodex, ID: "stable", AccountID: "legacy-subject", IDTokenChatGPTAccountID: "legacy-token-subject", Email: "person@example.com",
		Identity: ConnectionIdentity{DisplayName: "Production Codex", ExternalSubject: "subject-1", Attributes: map[string]string{"region": "us-east"}},
		PlanType: "pro", Totals: AccountUsage{RequestCount: 2},
		ResetCreditsAvailable: 1, ResetCreditsRetrievedAt: time.Now(),
		RateLimitResetCredits: []RateLimitResetCredit{{ID: "credit", ExpiresAt: time.Now().Add(time.Hour)}},
		Usage:                 UsageSnapshot{SecondaryUsedPercent: 0.99, SecondaryWindowMinutes: 10080, SecondaryResetAt: time.Now().Add(6 * time.Hour), RetrievedAt: time.Now(), Source: "wham", secondarySet: true},
	}
	handler := &proxyHandler{pool: newProviderPool([]*Account{connection})}
	recorder := httptest.NewRecorder()
	handler.serveProviderConnectionsV2(recorder)
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var views []OperatorProviderConnectionView
	if err := json.Unmarshal(recorder.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("view count=%d", len(views))
	}
	got := views[0]
	if got.ID != "stable" || got.PublicID != hashAccountID("stable") || got.ProviderID != AccountTypeCodex || got.Identity.DisplayName != "Production Codex" || got.Identity.ExternalSubject != "subject-1" || got.Identity.Attributes["email"] != "person@example.com" || got.Identity.Attributes["region"] != "us-east" || got.Totals.RequestCount != 2 {
		t.Fatalf("v2 view = %#v", got)
	}
	if got.Runtime.Status != "cooldown" || got.Runtime.StatusDetail != "Secondary quota is exhausted" || got.Runtime.SecondaryUsedPercent == nil || *got.Runtime.SecondaryUsedPercent != 99 || got.Runtime.SecondaryWindowMinutes == nil || *got.Runtime.SecondaryWindowMinutes != 10080 || got.Runtime.SecondaryResetAt == nil || got.Runtime.UsageRetrievedAt == nil || got.Runtime.UsageSource != "wham" {
		t.Fatalf("runtime projection = %#v", got.Runtime)
	}
	if !got.ResetCredits.Known || got.ResetCredits.AvailableCount != 1 || len(got.ResetCredits.Expirations) != 1 || !got.ResetCredits.ManagementAvailable || !got.ResetCredits.InventoryRefreshAvailable || !got.ResetCredits.RedemptionAvailable {
		t.Fatalf("reset credit projection = %#v", got.ResetCredits)
	}
	if got.ResetCredits.DashboardURL != codexResetCreditsDashboardURL {
		t.Fatalf("reset credit dashboard URL = %q", got.ResetCredits.DashboardURL)
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"account_id", "account_email", "id_token_chatgpt_account_id", "upstream_account_id"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("v2 response leaked compatibility field %q: %s", forbidden, body)
		}
	}
}

func TestProviderConnectionsV2ReturnsEmptyArray(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&proxyHandler{pool: newProviderPool(nil)}).serveProviderConnectionsV2(recorder)
	if strings.TrimSpace(recorder.Body.String()) != "[]" {
		t.Fatalf("body=%s, want []", recorder.Body.String())
	}
}
