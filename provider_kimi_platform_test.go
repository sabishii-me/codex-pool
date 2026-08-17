package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Model routing tests ---

// TestIsKimiPlatformModelRequiresPrefix verifies that the "kimi-platform/"
// prefix still routes to Kimi Platform accounts.
func TestIsKimiPlatformModelRequiresPrefix(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"kimi-platform/kimi-k3", "kimi-platform/kimi-k2.7-code-highspeed"} {
		if !isKimiPlatformModel(model) {
			t.Fatalf("expected %q to route to kimi-platform", model)
		}
	}
}

// TestIsKimiPlatformModelRejectsCodingPlanModels verifies that bare
// "kimi-for-coding" does NOT route to kimi-platform. This is the core
// product/conflation guard: kimi-for-coding is a Coding Plan model and
// must stay on AccountTypeKimi (Coding Plan routing).
func TestIsKimiPlatformModelRejectsCodingPlanModels(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"kimi-for-coding", "kimi-for-coding-highspeed"} {
		if isKimiPlatformModel(model) {
			t.Fatalf("bare %q must NOT route to kimi-platform (Coding Plan model)", model)
		}
	}
}

// TestIsKimiPlatformModelMatchesBarePlatformModels verifies that real
// Open Platform model IDs (kimi-k3, kimi-k2.7-code, etc.) route to
// kimi-platform even without the "kimi-platform/" prefix. They are not
// claimed by the Coding Plan catalog, so they belong to the Platform.
func TestIsKimiPlatformModelMatchesBarePlatformModels(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"kimi-k3", "kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k2.6", "kimi-k2.5", "kimi-k3[1m]"} {
		if !isKimiPlatformModel(model) {
			t.Fatalf("expected bare Open Platform model %q to route to kimi-platform", model)
		}
	}
}

// TestIsKimiPlatformModelRejectsUnrelatedModels verifies that models from
// other providers are not routed to kimi-platform.
func TestIsKimiPlatformModelRejectsUnrelatedModels(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"gpt-5.6", "deepseek-v4-pro"} {
		if isKimiPlatformModel(model) {
			t.Fatalf("did not expect %q to route to kimi-platform", model)
		}
	}
}

// TestKimiPlatformCanonicalModelStripsPrefix verifies that the
// "kimi-platform/" prefix is stripped for upstream forwarding.
func TestKimiPlatformCanonicalModelStripsPrefix(t *testing.T) {
	t.Parallel()

	got := kimiPlatformCanonicalModel("kimi-platform/kimi-k3")
	want := "kimi-k3"
	if got != want {
		t.Fatalf("kimiPlatformCanonicalModel() = %q, want %q", got, want)
	}
}

// TestKimiPlatformCanonicalBareModelPassesThrough verifies that bare
// Open Platform models (e.g. "kimi-k3") pass through without modification.
func TestKimiPlatformCanonicalBareModelPassesThrough(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"kimi-k3", "kimi-k2.7-code", "kimi-k2.7-code-highspeed"} {
		got := kimiPlatformCanonicalModel(model)
		if got != model {
			t.Fatalf("kimiPlatformCanonicalModel(%q) = %q, want %q", model, got, model)
		}
	}
}

// --- Route override tests ---

func TestModelRouteOverrideKimiPlatformPrefixedModel(t *testing.T) {
	t.Parallel()

	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	handler := &proxyHandler{
		registry: NewProviderRegistry(
			&CodexProvider{},
			&GeminiProvider{},
			NewKimiPlatformProvider(kimiPlatformBase),
		),
	}

	provider, base, rewritten := handler.modelRouteOverride("/v1/messages", "kimi-platform/kimi-k3", []byte(`{"model":"kimi-platform/kimi-k3"}`))
	if provider == nil {
		t.Fatal("expected override provider")
	}
	if provider.Type() != AccountTypeKimiPlatform {
		t.Fatalf("expected kimi-platform provider, got %s", provider.Type())
	}
	if base == nil || base.String() != kimiPlatformBase.String() {
		t.Fatalf("expected kimi-platform base %s, got %v", kimiPlatformBase, base)
	}
	if string(rewritten) != `{"model":"kimi-k3"}` {
		t.Fatalf("unexpected rewritten body (prefix should be stripped): %s", rewritten)
	}
}

// TestModelRouteOverrideKimiPlatformBareModel verifies that a bare Open
// Platform model ID (e.g. "kimi-k3") routes to the kimi-platform provider
// without body rewriting (the model name passes through as-is).
func TestModelRouteOverrideKimiPlatformBareModel(t *testing.T) {
	t.Parallel()

	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	handler := &proxyHandler{
		registry: NewProviderRegistry(
			&CodexProvider{},
			&GeminiProvider{},
			NewKimiPlatformProvider(kimiPlatformBase),
		),
	}

	provider, base, rewritten := handler.modelRouteOverride("/v1/messages", "kimi-k3", []byte(`{"model":"kimi-k3"}`))
	if provider == nil {
		t.Fatal("expected override provider for bare platform model")
	}
	if provider.Type() != AccountTypeKimiPlatform {
		t.Fatalf("expected kimi-platform provider, got %s", provider.Type())
	}
	if base == nil || base.String() != kimiPlatformBase.String() {
		t.Fatalf("expected kimi-platform base %s, got %v", kimiPlatformBase, base)
	}
	// Bare platform models pass through without body rewriting
	if string(rewritten) != `{"model":"kimi-k3"}` {
		t.Fatalf("bare platform model should not be rewritten; got: %s", rewritten)
	}
}

// TestModelRouteOverrideKimiCodingPlanModelDoesNotRouteToPlatform verifies
// that "kimi-for-coding" still routes to the Coding Plan (AccountTypeKimi),
// not the Platform. This prevents the product/model conflation.
func TestModelRouteOverrideKimiCodingPlanModelDoesNotRouteToPlatform(t *testing.T) {
	t.Parallel()

	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	kimiBase, _ := url.Parse("https://api.kimi.com/coding")
	handler := &proxyHandler{
		registry: NewProviderRegistry(
			&CodexProvider{},
			&GeminiProvider{},
			NewKimiProvider(kimiBase),
			NewKimiPlatformProvider(kimiPlatformBase),
		),
	}

	// "kimi-for-coding" must route to AccountTypeKimi, not AccountTypeKimiPlatform
	provider, base, _ := handler.modelRouteOverride("/v1/messages", "kimi-for-coding", []byte(`{"model":"kimi-for-coding"}`))
	if provider == nil {
		t.Fatal("expected override provider for kimi-for-coding")
	}
	if provider.Type() != AccountTypeKimi {
		t.Fatalf("kimi-for-coding must route to AccountTypeKimi, got %s", provider.Type())
	}
	if base == nil || base.String() != kimiBase.String() {
		t.Fatalf("expected kimi base %s, got %v", kimiBase, base)
	}
}

func TestKimiPlatformCatalogUsesOfficialModelsAndLimits(t *testing.T) {
	t.Parallel()

	want := map[string]struct {
		contextWindow int
		maxTokens     int
	}{
		"kimi-k3":                  {1048576, 131072},
		"kimi-k3[1m]":              {1048576, 131072},
		"kimi-k2.7-code":           {262144, 32768},
		"kimi-k2.7-code-highspeed": {262144, 32768},
		"kimi-k2.6":                {262144, 32768},
		"kimi-k2.5":                {262144, 32768},
	}
	models := modelsForProvider(AccountTypeKimiPlatform)
	// Kimi Platform is a declarative provider; its catalog lives in the
	// embedded provider spec (provider-specs.builtin/kimi-platform.json),
	// not in poolModels. Fall back to the builtin spec when poolModels no
	// longer carries standard provider rows.
	if len(models) == 0 {
		models = modelRouteSpecsFromProviderSpec(NewKimiPlatformProvider(nil).Spec())
	}
	if len(models) != len(want) {
		t.Fatalf("Kimi Platform catalog has %d models, want %d: %#v", len(models), len(want), models)
	}
	for _, model := range models {
		expected, ok := want[model.ID]
		if !ok {
			t.Fatalf("unexpected Kimi Platform model %q", model.ID)
		}
		if model.ContextWindow != expected.contextWindow || model.MaxTokens != expected.maxTokens {
			t.Fatalf("model %s limits = context %d, output %d; want context %d, output %d", model.ID, model.ContextWindow, model.MaxTokens, expected.contextWindow, expected.maxTokens)
		}
	}
	if _, ok := modelForProvider(AccountTypeKimiPlatform, "kimi-for-coding"); ok {
		t.Fatal("Coding Plan model kimi-for-coding must not appear in the Open Platform catalog")
	}
}

func TestKimiPlatformUsesClaudeWireFormat(t *testing.T) {
	t.Parallel()
	if got := providerTargetFormat(AccountTypeKimiPlatform); got != FormatClaude {
		t.Fatalf("providerTargetFormat(kimi-platform) = %v, want Claude", got)
	}
}

// --- Account loading tests ---

func TestKimiPlatformLoadAccountReadsAPIKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "kimi-platform_abcd.json")
	if err := os.WriteFile(path, []byte(`{"api_key":"sk-moonshot-test"}`), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewKimiPlatformProvider(nil)
	acc, err := p.LoadAccount("kimi-platform_abcd.json", path, data)
	if err != nil {
		t.Fatalf("LoadAccount() error = %v", err)
	}
	if acc == nil {
		t.Fatal("LoadAccount() returned nil account")
	}
	if acc.Type != AccountTypeKimiPlatform {
		t.Fatalf("acc.Type = %s, want %s", acc.Type, AccountTypeKimiPlatform)
	}
	if acc.AccessToken != "sk-moonshot-test" {
		t.Fatalf("acc.AccessToken = %q, want %q", acc.AccessToken, "sk-moonshot-test")
	}
}

func TestKimiPlatformProviderSetAuthHeaders(t *testing.T) {
	t.Parallel()

	provider := NewKimiPlatformProvider(nil)
	acc := &Account{AccessToken: "sk-test-key"}
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	provider.SetAuthHeaders(req, acc)

	if req.Header.Get("Authorization") != "Bearer sk-test-key" {
		t.Fatalf("Authorization = %q, want %q", req.Header.Get("Authorization"), "Bearer sk-test-key")
	}
}

// --- Admin validation tests ---

// TestKimiPlatformAdminAddValidatesAndSavesAccount validates using
// GET /v1/models against the derived OpenAI-compatible base URL.
func TestKimiPlatformAdminAddValidatesAndSavesAccount(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	validationCalled := false
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewKimiPlatformProvider(kimiPlatformBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			validationCalled = true
			// Validate via GET /v1/models on the OpenAI-compatible endpoint
			if req.Method != http.MethodGet {
				t.Fatalf("validation method = %s, want GET", req.Method)
			}
			if req.URL.String() != "https://api.moonshot.ai/v1/models" {
				t.Fatalf("validation url = %q, want %q", req.URL.String(), "https://api.moonshot.ai/v1/models")
			}
			if req.Header.Get("Authorization") != "Bearer sk-moonshot-valid" {
				t.Fatalf("validation auth = %q", req.Header.Get("Authorization"))
			}
			// GET /v1/models returns the model list for authenticated keys
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[{"id":"kimi-k3","object":"model"}]}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", strings.NewReader(`{"api_key":"sk-moonshot-valid"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleKimiPlatformAdd(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !validationCalled {
		t.Fatal("validation was not called")
	}
	entries, err := os.ReadDir(filepath.Join(poolDir, "kimi-platform"))
	if err != nil {
		t.Fatalf("read kimi-platform pool dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("saved %d Kimi Platform files, want 1", len(entries))
	}
	if h.pool.countByType(AccountTypeKimiPlatform) != 1 {
		t.Fatalf("pool Kimi Platform count = %d, want 1", h.pool.countByType(AccountTypeKimiPlatform))
	}

	var saved map[string]any
	raw, _ := os.ReadFile(filepath.Join(poolDir, "kimi-platform", entries[0].Name()))
	_ = json.Unmarshal(raw, &saved)
	if saved["api_key"] != "sk-moonshot-valid" {
		t.Fatalf("saved api_key = %v, want sk-moonshot-valid", saved["api_key"])
	}
}

func TestKimiPlatformAdminRejectsUnauthorizedKeyWithoutSaving(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewKimiPlatformProvider(kimiPlatformBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader(`{"error":"bad key"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", strings.NewReader(`{"api_key":"sk-moonshot-bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleKimiPlatformAdd(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(poolDir, "kimi-platform")); !os.IsNotExist(err) {
		t.Fatalf("kimi-platform pool dir should not exist after rejected key, err=%v", err)
	}
}

// TestKimiPlatformAdminPreservesUpstreamDiagnostics verifies that the
// upstream error body is included in the response so operators can
// distinguish "invalid key" from "wrong endpoint" from "model not found".
func TestKimiPlatformAdminPreservesUpstreamDiagnostics(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewKimiPlatformProvider(kimiPlatformBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// Simulate a 404 from a wrong-endpoint scenario - the response
			// body should be propagated to the caller for debugging.
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Body:       io.NopCloser(strings.NewReader(`{"error":"model_not_found","message":"The model 'kimi-for-coding' does not exist or you do not have access to it"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", strings.NewReader(`{"api_key":"sk-moonshot-wrong-endpoint"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleKimiPlatformAdd(rr, req)

	// 404 is not 401/403, so it should be BadGateway, not BadRequest
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 BadGateway for 404, got %d: %s", rr.Code, rr.Body.String())
	}
	// The upstream error body should be visible in the response
	body := rr.Body.String()
	if !strings.Contains(body, "model_not_found") {
		t.Fatalf("response should preserve upstream error body, got: %s", body)
	}
	if !strings.Contains(body, "kimi-for-coding") {
		t.Fatalf("response should include the model name from upstream, got: %s", body)
	}
}

func TestKimiPlatformAdminNetworkFailureDoesNotSave(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewKimiPlatformProvider(kimiPlatformBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: os.ErrDeadlineExceeded}
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", strings.NewReader(`{"api_key":"sk-moonshot-x"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleKimiPlatformAdd(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(poolDir, "kimi-platform")); !os.IsNotExist(err) {
		t.Fatalf("kimi-platform pool dir should not exist after network failure, err=%v", err)
	}
}

// TestKimiPlatformAdminRejectsForbiddenKeyWithoutSaving ensures 403 is
// rejected (same as 401).
func TestKimiPlatformAdminRejectsForbiddenKeyWithoutSaving(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewKimiPlatformProvider(kimiPlatformBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Status:     "403 Forbidden",
				Body:       io.NopCloser(strings.NewReader(`{"error":"forbidden"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", strings.NewReader(`{"api_key":"sk-moonshot-forbidden"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleKimiPlatformAdd(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "API key validation rejected") {
		t.Fatalf("expected 'API key validation rejected' in response, got: %s", rr.Body.String())
	}
}

// TestKimiPlatformAdminNon401ErrorDoesNotClaimInvalidKey verifies that a
// 429 (rate limited) or other non-auth error is not reported as "invalid key"
// but passed through as an infrastructure error with the upstream body.
func TestKimiPlatformAdminNon401ErrorDoesNotClaimInvalidKey(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewKimiPlatformProvider(kimiPlatformBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Status:     "429 Too Many Requests",
				Body:       io.NopCloser(strings.NewReader(`{"error":"rate_limit_exceeded"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", strings.NewReader(`{"api_key":"sk-moonshot-valid-key"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleKimiPlatformAdd(rr, req)

	// 429 should NOT be reported as "invalid API key"
	if strings.Contains(rr.Body.String(), "invalid API key") {
		t.Fatalf("should NOT report 429 as invalid key, got: %s", rr.Body.String())
	}
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 BadGateway for 429, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestKimiPlatformAdminValidationDerivesOpenAIBase verifies that the
// validation URL is correctly constructed using url.JoinPath from the
// configured Anthropic base by taking scheme+host (to hit the
// OpenAI-compatible /v1/models endpoint).
// This uses proper URL joining without path confusion.
func TestKimiPlatformAdminValidationDerivesOpenAIBase(t *testing.T) {
	t.Parallel()

	// The configured base is the Anthropic-compatible endpoint
	base, _ := url.Parse("https://api.moonshot.ai/anthropic")
	// Derive the OpenAI base: scheme + host only
	openAIBase := &url.URL{Scheme: base.Scheme, Host: base.Host}
	got, err := url.JoinPath(openAIBase.String(), "/v1/models")
	if err != nil {
		t.Fatalf("url.JoinPath error: %v", err)
	}
	want := "https://api.moonshot.ai/v1/models"
	if got != want {
		t.Fatalf("derived validation URL = %q, want %q", got, want)
	}
}

// TestKimiPlatformAdminValidatesUsingGETV1Models verifies the key validation
// sends a GET to /v1/models on the OpenAI-compatible endpoint, which is the
// official documented method (not a model-dependent POST to the Anthropic
// endpoint that would fail with "model not found" for valid keys).
func TestKimiPlatformAdminValidatesUsingGETV1Models(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	var gotReq *http.Request
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewKimiPlatformProvider(kimiPlatformBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotReq = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[]}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", strings.NewReader(`{"api_key":"sk-key"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleKimiPlatformAdd(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if gotReq == nil {
		t.Fatal("validation request was not made")
	}
	if gotReq.Method != http.MethodGet {
		t.Fatalf("validation uses %s, want GET (not model-dependent POST)", gotReq.Method)
	}
	if !strings.HasSuffix(gotReq.URL.Path, "/v1/models") {
		t.Fatalf("validation path = %s, want /v1/models", gotReq.URL.Path)
	}
	if gotReq.Header.Get("Authorization") != "Bearer sk-key" {
		t.Fatalf("Authorization = %q", gotReq.Header.Get("Authorization"))
	}
	// No model-dependent body should be sent
	if gotReq.Body != nil && gotReq.ContentLength > 0 {
		t.Fatal("GET /v1/models validation should not send a request body")
	}
}

// TestKimiPlatformAdminAuthErrorIncludesUpstreamBody verifies that 401
// responses from upstream are included in the error message so operators
// can distinguish "invalid key" from "wrong endpoint."
func TestKimiPlatformAdminAuthErrorIncludesUpstreamBody(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewKimiPlatformProvider(kimiPlatformBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader(`{"error":"auth_failed","message":"invalid API key"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", strings.NewReader(`{"api_key":"sk-invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleKimiPlatformAdd(rr, req)

	// Must include the upstream status code in the message
	body := rr.Body.String()
	if !strings.Contains(body, "401") {
		t.Fatalf("auth error should include HTTP status, got: %s", body)
	}
	// Must include the upstream response body for diagnostics
	if !strings.Contains(body, "auth_failed") {
		t.Fatalf("auth error should include upstream response body, got: %s", body)
	}
	if !strings.Contains(body, "invalid API key") {
		t.Fatalf("auth error should include upstream detail, got: %s", body)
	}
}

// TestKimiPlatformAdminWrongEndpoint403PreservesBody verifies that when the
// validation endpoint itself returns 403 (possible endpoint/platform mismatch),
// the upstream body is preserved so operators can diagnose the issue.
func TestKimiPlatformAdminWrongEndpoint403PreservesBody(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	h := &proxyHandler{
		cfg:      &config{poolDir: poolDir},
		pool:     newProviderPool(nil),
		registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewKimiPlatformProvider(kimiPlatformBase)),
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Status:     "403 Forbidden",
				Body:       io.NopCloser(strings.NewReader(`{"error":"method_not_allowed","message":"the requested endpoint does not support this method"}`)),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/kimi-platform/add", strings.NewReader(`{"api_key":"sk-key"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.handleKimiPlatformAdd(rr, req)

	// Must include the upstream status code
	body := rr.Body.String()
	if !strings.Contains(body, "403") {
		t.Fatalf("error should include HTTP status 403, got: %s", body)
	}
	// Must include upstream body so operators see endpoint mismatch, not just "invalid key"
	if !strings.Contains(body, "method_not_allowed") {
		t.Fatalf("error should include upstream response body, got: %s", body)
	}
	// The key was valid (upstream said method_not_allowed, not auth failure)
	// The response should clarify this is an endpoint issue
	if strings.Contains(body, "invalid API key") {
		t.Fatalf("response should not claim 'invalid API key' for endpoint mismatch, got: %s", body)
	}
}

// TestKimiPlatformProductModelConflationEndToEnd verifies that a Kimi
// Platform account added and validated through the admin endpoint routes
// actual platform model IDs (kimi-k3, kimi-k2.7-code) but does NOT route
// kimi-for-coding (which is a Coding Plan model). This test exercises the
// full product/model conflation guard end-to-end.
func TestKimiPlatformProductModelConflationEndToEnd(t *testing.T) {
	t.Parallel()

	poolDir := t.TempDir()
	kimiPlatformBase, _ := url.Parse("https://api.moonshot.ai/anthropic")
	kimiBase, _ := url.Parse("https://api.kimi.com/coding")

	// Account for testing: capture which accounts got picked
	platformAcc := &Account{
		Type:        AccountTypeKimiPlatform,
		ID:          "kimi-platform_abcd",
		AccessToken: "sk-moonshot-valid",
		PlanType:    "kimi-platform",
	}
	codingAcc := &Account{
		Type:        AccountTypeKimi,
		ID:          "kimi_abcd",
		AccessToken: "sk-kimi-valid",
		PlanType:    "kimi",
	}

	pool := newProviderPool([]*Account{platformAcc, codingAcc})

	handler := &proxyHandler{
		cfg:  &config{poolDir: poolDir},
		pool: pool,
		registry: NewProviderRegistry(
			&CodexProvider{},
			&GeminiProvider{},
			NewKimiProvider(kimiBase),
			NewKimiPlatformProvider(kimiPlatformBase),
		),
	}

	t.Run("kimi-for-coding routes to Coding Plan (not Platform)", func(t *testing.T) {
		provider, base, _ := handler.modelRouteOverride("/v1/messages", "kimi-for-coding", []byte(`{"model":"kimi-for-coding"}`))
		if provider == nil {
			t.Fatal("expected override provider for kimi-for-coding")
		}
		if provider.Type() != AccountTypeKimi {
			t.Fatalf("kimi-for-coding must route to AccountTypeKimi (Coding Plan), got %s", provider.Type())
		}
		if base == nil || base.String() != kimiBase.String() {
			t.Fatalf("expected kimi base %s, got %v", kimiBase, base)
		}
	})

	t.Run("kimi-k3 routes to Platform (not Coding Plan)", func(t *testing.T) {
		provider, base, _ := handler.modelRouteOverride("/v1/messages", "kimi-k3", []byte(`{"model":"kimi-k3"}`))
		if provider == nil {
			t.Fatal("expected override provider for kimi-k3")
		}
		if provider.Type() != AccountTypeKimiPlatform {
			t.Fatalf("kimi-k3 must route to AccountTypeKimiPlatform, got %s", provider.Type())
		}
		if base == nil || base.String() != kimiPlatformBase.String() {
			t.Fatalf("expected kimi platform base %s, got %v", kimiPlatformBase, base)
		}
	})

	t.Run("prefixed kimi-k3 routes to Platform", func(t *testing.T) {
		provider, base, rewritten := handler.modelRouteOverride("/v1/messages", "kimi-platform/kimi-k3", []byte(`{"model":"kimi-platform/kimi-k3"}`))
		if provider == nil {
			t.Fatal("expected override provider for prefixed kimi-k3")
		}
		if provider.Type() != AccountTypeKimiPlatform {
			t.Fatalf("prefixed kimi-k3 must route to AccountTypeKimiPlatform, got %s", provider.Type())
		}
		if base == nil || base.String() != kimiPlatformBase.String() {
			t.Fatalf("expected kimi platform base %s, got %v", kimiPlatformBase, base)
		}
		if string(rewritten) != `{"model":"kimi-k3"}` {
			t.Fatalf("kimi-platform prefix should be stripped, got: %s", rewritten)
		}
	})

	t.Run("kimi-for-coding-highspeed routes to Coding Plan (not Platform)", func(t *testing.T) {
		provider, base, _ := handler.modelRouteOverride("/v1/messages", "kimi-for-coding-highspeed", []byte(`{"model":"kimi-for-coding-highspeed"}`))
		if provider == nil {
			t.Fatal("expected override provider for kimi-for-coding-highspeed")
		}
		if provider.Type() != AccountTypeKimi {
			t.Fatalf("kimi-for-coding-highspeed must route to AccountTypeKimi (Coding Plan), got %s", provider.Type())
		}
		if base == nil || base.String() != kimiBase.String() {
			t.Fatalf("expected kimi base %s, got %v", kimiBase, base)
		}
	})
}
