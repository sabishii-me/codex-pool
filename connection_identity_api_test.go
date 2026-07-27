package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestPoolStatsExposesProviderNeutralConnectionIdentity(t *testing.T) {
	account := &Account{
		Type: AccountTypeCodex, ID: "stable-connection", AccountID: "upstream-subject", Email: "person@example.com",
		Identity: ConnectionIdentity{DisplayName: "Production Codex", Attributes: map[string]string{"region": "us-east"}},
	}
	handler := &proxyHandler{pool: newProviderPool([]*Account{account})}
	recorder := httptest.NewRecorder()
	handler.handlePoolStats(recorder, httptest.NewRequest("GET", "/api/pool/stats", nil))
	var payload struct {
		Accounts []AccountStats `json:"accounts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Accounts) != 1 {
		t.Fatalf("account count = %d", len(payload.Accounts))
	}
	got := payload.Accounts[0]
	if got.DisplayName != "Production Codex" || got.ExternalSubject != "upstream-subject" || got.IdentityAttributes["email"] != "person@example.com" || got.IdentityAttributes["region"] != "us-east" {
		t.Fatalf("stats identity = %#v", got)
	}
	if got.ID != hashAccountID(account.ID) {
		t.Fatalf("public ID changed: %q", got.ID)
	}
	if got.UpstreamAccountID != "upstream-subject" || got.AccountEmail != "person@example.com" {
		t.Fatalf("compatibility identity fields missing: %#v", got)
	}
}

func TestOperatorAccountsExposesProviderNeutralConnectionIdentity(t *testing.T) {
	account := &Account{
		Type: AccountTypeAntigravity, ID: "connection", Email: "operator@example.com", ProjectID: "project-1",
		Identity: ConnectionIdentity{DisplayName: "Antigravity Workspace", ExternalSubject: "workspace-1"},
	}
	handler := &proxyHandler{pool: newProviderPool([]*Account{account})}
	recorder := httptest.NewRecorder()
	handler.serveAccounts(recorder)
	var rows []struct {
		ID                 string            `json:"id"`
		PublicID           string            `json:"public_id"`
		DisplayName        string            `json:"display_name"`
		ExternalSubject    string            `json:"external_subject"`
		IdentityAttributes map[string]string `json:"identity_attributes"`
		Email              string            `json:"email"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count = %d", len(rows))
	}
	got := rows[0]
	if got.ID != account.ID || got.PublicID != hashAccountID(account.ID) || got.DisplayName != "Antigravity Workspace" || got.ExternalSubject != "workspace-1" || got.IdentityAttributes["email"] != "operator@example.com" || got.IdentityAttributes["project_id"] != "project-1" {
		t.Fatalf("operator identity = %#v", got)
	}
	if got.Email != "operator@example.com" {
		t.Fatalf("legacy email field missing: %#v", got)
	}
}
