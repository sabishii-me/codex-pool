package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalDomainVocabularySharesCompatibilityIdentity(t *testing.T) {
	connection := &ProviderConnection{Type: ProviderID("codex"), ID: "connection"}
	var legacyAccount *Account = connection
	if legacyAccount.ID != "connection" {
		t.Fatal("ProviderConnection compatibility alias changed identity")
	}
	pool := &ProviderPool{accounts: []*ProviderConnection{connection}}
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

func TestProductionSourceUsesProviderConnectionVocabulary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "pool.go" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, data, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if ident, ok := node.(*ast.Ident); ok && ident.Name == "Account" {
				t.Errorf("%s uses deprecated Account identifier; use ProviderConnection", path)
			}
			return true
		})
	}
}
