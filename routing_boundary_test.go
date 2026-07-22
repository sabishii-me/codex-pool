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

func TestProductionRequestPathsUseRoutingBoundaries(t *testing.T) {
	allowedDirectPoolFiles := map[string]bool{
		"pool.go": true, "antigravity_models.go": true, "connection_selector.go": true,
	}
	forbiddenPoolMethods := map[string]bool{
		"candidate": true, "candidateByID": true, "candidateWithCyberAccess": true,
		"candidateForAntigravityModel": true, "imageFanoutCandidate": true, "nearestCooldown": true,
		"excludeImageIncapable": true, "excludeInflightWhenIdleAvailable": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || allowedDirectPoolFiles[filepath.Base(path)] {
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
			selector, ok := node.(*ast.SelectorExpr)
			if ok && forbiddenPoolMethods[selector.Sel.Name] {
				t.Errorf("%s calls selection internal %s directly; use ConnectionSelector", path, selector.Sel.Name)
			}
			return true
		})
	}
}

func TestReadOnlyDataRoutesStayOutOfProxyRouter(t *testing.T) {
	data, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		`case "/api/pool/stats"`,
		`case "/api/pool/users"`,
		`case "/api/pool/origins"`,
		`case "/api/pool/daily-breakdown"`,
		`case "/api/pool/hourly"`,
		`case "/api/pool/signal"`,
		`case "/api/pool/catalog"`,
		`case "/api/v2/provider-connections"`,
		`case "/admin/accounts"`,
	} {
		if strings.Contains(string(data), path) {
			t.Errorf("router.go directly owns read route %s; use DataAPI", path)
		}
	}
	if !strings.Contains(string(data), "h.dataAPIService().TryServe(w, r)") {
		t.Fatal("proxy router does not delegate to DataAPI")
	}
}

func TestAccessPolicyDecisionsStayOutOfProxyRouter(t *testing.T) {
	data, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, decision := range []string{"adminEmailAllowed(", "adminElevated(", ".isBanned(", ".recordFailure(", ".recordSuccess("} {
		if strings.Contains(string(data), decision) {
			t.Errorf("router.go contains access decision %q; use AccessPolicy", decision)
		}
	}
}

func TestProductionModelRoutingIsCentralized(t *testing.T) {
	allowed := map[string]bool{
		"model_route_registry.go": true, "provider.go": true, "provider_grok.go": true,
		"provider_kimi.go": true, "antigravity_translate.go": true,
	}
	forbidden := map[string]bool{
		"shouldRouteAntigravityModel": true, "isGrokModel": true, "isKimiModel": true, "MatchDeclarativeModel": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || allowed[filepath.Base(path)] {
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
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch function := call.Fun.(type) {
			case *ast.Ident:
				if forbidden[function.Name] {
					t.Errorf("%s calls model matcher %s directly; use ModelRouteRegistry", path, function.Name)
				}
			case *ast.SelectorExpr:
				if forbidden[function.Sel.Name] {
					t.Errorf("%s calls model matcher %s directly; use ModelRouteRegistry", path, function.Sel.Name)
				}
			}
			return true
		})
	}
}
