package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameProviderConnectionPersistsWithoutChangingStableID(t *testing.T) {
	file := filepath.Join(t.TempDir(), "connection.json")
	if err := os.WriteFile(file, []byte(`{"api_key":"secret","display_name":"Old name"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	account := &Account{Type: AccountTypeDeepSeek, ID: "stable-id", File: file, AccessToken: "secret", Identity: ConnectionIdentity{DisplayName: "Old name"}, Label: "Old name"}
	handler := &proxyHandler{pool: newProviderPool([]*Account{account})}
	request := httptest.NewRequest(http.MethodPatch, "/admin/accounts/stable-id/identity", bytes.NewBufferString(`{"display_name":"New operator label"}`))
	recorder := httptest.NewRecorder()
	handler.renameProviderConnection(recorder, request, account.ID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if account.ID != "stable-id" || account.Identity.DisplayName != "New operator label" || account.Label != "New operator label" {
		t.Fatalf("renamed account = ID %q identity %#v label %q", account.ID, account.Identity, account.Label)
	}
	var saved map[string]any
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["display_name"] != "New operator label" || saved["api_key"] != "secret" {
		t.Fatalf("saved connection = %#v", saved)
	}
}

func TestRenameProviderConnectionValidatesDisplayName(t *testing.T) {
	account := &Account{Type: AccountTypeCodex, ID: "stable", Identity: ConnectionIdentity{DisplayName: "Original"}, Label: "Original"}
	handler := &proxyHandler{pool: newProviderPool([]*Account{account})}
	for name, payload := range map[string]string{
		"empty":   `{"display_name":"  "}`,
		"control": `{"display_name":"bad\nname"}`,
		"long":    `{"display_name":"` + strings.Repeat("x", 121) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.renameProviderConnection(recorder, httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString(payload)), account.ID)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if account.Identity.DisplayName != "Original" || account.ID != "stable" {
				t.Fatalf("invalid rename mutated connection: %#v", account)
			}
		})
	}
}

func TestRenameProviderConnectionRollsBackOnPersistenceFailure(t *testing.T) {
	account := &Account{Type: AccountTypeDeepSeek, ID: "stable", File: filepath.Join(t.TempDir(), "missing", "connection.json"), Identity: ConnectionIdentity{DisplayName: "Original"}, Label: "Original"}
	handler := &proxyHandler{pool: newProviderPool([]*Account{account})}
	recorder := httptest.NewRecorder()
	handler.renameProviderConnection(recorder, httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString(`{"display_name":"Changed"}`)), account.ID)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if account.Identity.DisplayName != "Original" || account.Label != "Original" {
		t.Fatalf("failed persistence did not roll back: %#v", account.Identity)
	}
}
