package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConnectionIdentityLegacyFallbackIsProviderNeutral(t *testing.T) {
	account := &Account{Type: AccountTypeCodex, ID: "stable-connection-id", AccountID: "acct_1234567890abcdef", Email: "person@example.com"}
	identity := account.ConnectionIdentity()
	if identity.DisplayName != "person@example.com" {
		t.Fatalf("display name = %q", identity.DisplayName)
	}
	if identity.ExternalSubject != "acct_1234567890abcdef" {
		t.Fatalf("external subject = %q", identity.ExternalSubject)
	}
	if identity.Attributes["email"] != "person@example.com" {
		t.Fatalf("attributes = %#v", identity.Attributes)
	}
	if account.ID != "stable-connection-id" {
		t.Fatalf("identity derivation changed stable ID to %q", account.ID)
	}
}

func TestConnectionIdentityWithoutEmailUsesProviderAndAbbreviatedSubject(t *testing.T) {
	account := &Account{Type: AccountTypeGemini, ID: "gemini-file", AccountUUID: "1234567890abcdef"}
	identity := account.ConnectionIdentity()
	if identity.DisplayName != "gemini 123456…cdef" {
		t.Fatalf("display name = %q", identity.DisplayName)
	}
	if identity.ExternalSubject != "1234567890abcdef" {
		t.Fatalf("external subject = %q", identity.ExternalSubject)
	}
}

func TestConnectionIdentityLoadsAndPersistsDurableRename(t *testing.T) {
	file := filepath.Join(t.TempDir(), "connection.json")
	initial := []byte(`{"api_key":"secret","display_name":"Primary EU connection","external_subject":"tenant-1","identity_attributes":{"region":"eu-west","workspace":"core"}}`)
	if err := os.WriteFile(file, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	account := &Account{Type: AccountTypeDeepSeek, ID: "deepseek-stable", File: file, AccessToken: "secret"}
	applyCommonAccountFileState(account, initial)
	if account.Identity.DisplayName != "Primary EU connection" || account.Label != "Primary EU connection" || account.Identity.ExternalSubject != "tenant-1" || account.Identity.Attributes["region"] != "eu-west" {
		t.Fatalf("loaded identity = %#v label=%q", account.Identity, account.Label)
	}
	account.Identity.DisplayName = "Renamed connection"
	if err := saveAccount(account); err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["display_name"] != "Renamed connection" || saved["external_subject"] != "tenant-1" {
		t.Fatalf("saved identity = %#v", saved)
	}
	attributes := saved["identity_attributes"].(map[string]any)
	if attributes["region"] != "eu-west" || attributes["workspace"] != "core" {
		t.Fatalf("saved attributes = %#v", attributes)
	}
	if account.ID != "deepseek-stable" {
		t.Fatalf("rename changed stable connection ID to %q", account.ID)
	}
}

func TestConnectionIdentityIgnoresNonStringMetadata(t *testing.T) {
	account := &Account{Type: AccountTypeNvidia, ID: "nvidia"}
	applyCommonAccountFileState(account, []byte(`{"display_name":" GPU ","identity_attributes":{"region":" us-east ","secret":123,"empty":" "}}`))
	if account.Identity.DisplayName != "GPU" || len(account.Identity.Attributes) != 1 || account.Identity.Attributes["region"] != "us-east" {
		t.Fatalf("identity = %#v", account.Identity)
	}
}
