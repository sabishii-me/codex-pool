package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRefreshCodexResetCreditsRejectsEnvironmentWithoutProviderStateManagement(t *testing.T) {
	connection := &ProviderConnection{ID: "codex", Type: AccountTypeCodex, AccessToken: "token"}
	h := &proxyHandler{cfg: &config{disableRefresh: true}, pool: newProviderPool([]*ProviderConnection{connection}, false)}
	response := httptest.NewRecorder()
	h.refreshCodexResetCredits(response, httptest.NewRequest(http.MethodPost, "/", nil), connection.ID)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "provider-state management is unavailable") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRedeemCodexResetCreditRejectsReadOnlyGateway(t *testing.T) {
	connection := &ProviderConnection{ID: "codex", Type: AccountTypeCodex, RateLimitResetCredits: []RateLimitResetCredit{{ID: "secret-credit", ExpiresAt: time.Now().Add(time.Hour)}}}
	h := &proxyHandler{cfg: &config{disableRefresh: true}, pool: newProviderPool([]*ProviderConnection{connection}, false)}
	response := httptest.NewRecorder()
	h.redeemCodexResetCredit(response, httptest.NewRequest(http.MethodPost, "/", nil), connection.ID)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRedeemCodexResetCreditUsesBackendSelectedOwningCredit(t *testing.T) {
	base, _ := url.Parse("https://chatgpt.example/backend-api")
	now := time.Now()
	connection := &ProviderConnection{
		ID: "codex", Type: AccountTypeCodex, AccessToken: "token", AccountID: "account",
		ResetCreditsAvailable: 2,
		RateLimitResetCredits: []RateLimitResetCredit{{ID: "later", ExpiresAt: now.Add(2 * time.Hour)}, {ID: "earlier", ExpiresAt: now.Add(time.Hour)}},
	}
	requests := 0
	h := &proxyHandler{cfg: &config{whamBase: base}, pool: newProviderPool([]*ProviderConnection{connection}, false), transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), `"credit_id":"earlier"`) || strings.Contains(string(body), `"credit_id":"later"`) {
				t.Fatalf("consume body=%s", body)
			}
			return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":"ok","windows_reset":2}`))}, nil
		}
		if requests == 2 {
			return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"available_count":0,"credits":[]}`))}, nil
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"rate_limit":{"primary_window":{"used_percent":0,"reset_at":1893456000,"limit_window_seconds":18000},"secondary_window":{"used_percent":0,"reset_at":1893456000,"limit_window_seconds":604800}}}`))}, nil
	})}
	response := httptest.NewRecorder()
	h.redeemCodexResetCredit(response, httptest.NewRequest(http.MethodPost, "/", nil), connection.ID)
	if response.Code != http.StatusOK || requests != 3 || !strings.Contains(response.Body.String(), `"refresh_complete":true`) || !strings.Contains(response.Body.String(), `"dashboard_url":"`+codexResetCreditsDashboardURL+`"`) {
		t.Fatalf("status=%d requests=%d body=%s", response.Code, requests, response.Body.String())
	}
}
