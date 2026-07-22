package main

import "testing"

func TestCanonicalDomainVocabularySharesCompatibilityIdentity(t *testing.T) {
	connection := &ProviderConnection{Type: ProviderID("codex"), ID: "connection"}
	var legacyAccount *Account = connection
	if legacyAccount.ID != "connection" {
		t.Fatal("ProviderConnection compatibility alias changed identity")
	}
	pool := &ProviderPool{accounts: []*Account{connection}}
	var legacyPool *poolState = pool
	if len(legacyPool.accounts) != 1 {
		t.Fatal("ProviderPool compatibility alias changed pool semantics")
	}
	user := &GatewayUser{ID: "user"}
	var legacyUser *PoolUser = user
	if legacyUser.ID != "user" {
		t.Fatal("GatewayUser compatibility alias changed user identity")
	}
	route := ModelRoute{ProviderID: AccountTypeCodex, ID: "model"}
	var legacyRoute poolModel = route
	if legacyRoute.ProviderID != ProviderID("codex") {
		t.Fatal("ModelRoute compatibility alias changed provider identity")
	}
}
