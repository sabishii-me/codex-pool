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

func TestCodexFairnessPolicyStaysBehindConnectionSelector(t *testing.T) {
	selectorData, err := os.ReadFile("connection_selector.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(selectorData), "selectQuotaCompetitiveConnection") || !strings.Contains(string(selectorData), "cyberAccessRoutingWeight") {
		t.Fatal("ConnectionSelector does not own weighted quota-competitive Codex policy")
	}
	for _, path := range []string{"main.go", "router.go", "cyber_swap_ws.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range []string{"selectQuotaCompetitiveConnection", "cyberAccessRoutingWeight", "competitiveRoutingScoreWindow"} {
			if strings.Contains(string(data), fragment) {
				t.Errorf("%s bypasses ConnectionSelector fairness via %q", path, fragment)
			}
		}
	}
}

func TestPoolStatsHandlerUsesDetachedConnectionViews(t *testing.T) {
	data, err := os.ReadFile("frontend.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	start := strings.Index(source, "func (h *proxyHandler) handlePoolStats")
	end := strings.Index(source[start:], "\nfunc (h *proxyHandler) computeCyberPolicyStats")
	if start < 0 || end < 0 {
		t.Fatal("could not locate pool stats handler")
	}
	handler := source[start : start+end]
	for _, fragment := range []string{"allAccounts(", ".mu.Lock(", ".mu.RLock("} {
		if strings.Contains(handler, fragment) {
			t.Errorf("pool stats handler reads mutable connections via %q; use ConnectionViewService", fragment)
		}
	}
	if !strings.Contains(handler, "PoolStatsConnections(") {
		t.Fatal("pool stats handler does not consume detached ConnectionViewService snapshots")
	}
}

func TestSystemAdminRoutesStayOutOfProxyRouter(t *testing.T) {
	data, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"/metrics", "/admin/reload", "/admin/origins", "/admin/tokens", "/admin/clear-rate-limits", "/admin/purge-anonymous", "/admin/pool-users", "servePoolUsersAdmin("} {
		if strings.Contains(string(data), fragment) {
			t.Errorf("router.go directly owns system admin fragment %q; use SystemAdminAPI", fragment)
		}
	}
	if !strings.Contains(string(data), "h.systemAdminAPIService().TryServe(w, r)") {
		t.Fatal("proxy router does not delegate to SystemAdminAPI")
	}
}

func TestAuthenticationRoutesStayOutOfProxyRouter(t *testing.T) {
	data, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"/auth/login/google", "/auth/callback/google", "/auth/callback/codex", "/auth/logout", "/api/pool/session", "/api/admin/mfa/", "handleGoogleLoginStart(", "handleMFAVerify("} {
		if strings.Contains(string(data), fragment) {
			t.Errorf("router.go directly owns authentication fragment %q; use AuthenticationAPI", fragment)
		}
	}
	if !strings.Contains(string(data), "h.authenticationAPIService().TryServe(w, r)") {
		t.Fatal("proxy router does not delegate to AuthenticationAPI")
	}
}

func TestProviderOperationRoutesStayOutOfProxyRouter(t *testing.T) {
	data, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"/admin/claude", "/admin/codex", "/admin/antigravity", "/admin/kimi", "/admin/deepseek", "serveCodexAdmin(", "handleAntigravityModelSync("} {
		if strings.Contains(string(data), fragment) {
			t.Errorf("router.go directly owns provider operation fragment %q; use ProviderOperationsAPI", fragment)
		}
	}
	if !strings.Contains(string(data), "h.providerOperationsAPIService().TryServe(w, r)") {
		t.Fatal("proxy router does not delegate to ProviderOperationsAPI")
	}
}

func TestProviderContributionRoutesStayOutOfProxyRouter(t *testing.T) {
	data, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"/api/pool/accounts/", "handleCodexAdd(", "handleDeepSeekAdd(", "handleNvidiaAdd("} {
		if strings.Contains(string(data), fragment) {
			t.Errorf("router.go directly owns contribution fragment %q; use ProviderContributionAPI", fragment)
		}
	}
	if !strings.Contains(string(data), "h.providerContributionAPIService().TryServe(w, r)") {
		t.Fatal("proxy router does not delegate to ProviderContributionAPI")
	}
}

func TestProviderLifecycleRoutesStayOutOfProxyRouter(t *testing.T) {
	data, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"/api/v2/provider-connections/", "/admin/accounts/", "renameProviderConnection(", "setAccountDisabled(", "resurrectAccount(", "forceRefreshAccount("} {
		if strings.Contains(string(data), fragment) {
			t.Errorf("router.go directly owns provider lifecycle fragment %q; use ProviderAdminAPI", fragment)
		}
	}
	if !strings.Contains(string(data), "h.providerAdminAPIService().TryServe(w, r)") {
		t.Fatal("proxy router does not delegate to ProviderAdminAPI")
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
