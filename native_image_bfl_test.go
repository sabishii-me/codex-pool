package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeImageRegistryDoesNotInferFromVisionOrNames(t *testing.T) {
	if _, ok := resolveNativeModel("gpt-5.6-luna", WorkloadImageGeneration); ok {
		t.Fatal("vision/tool LLM entered native image pool")
	}
	if _, ok := resolveNativeModel("antigravity/gemini-3.1-flash-image", WorkloadImageGeneration); ok {
		t.Fatal("coding-agent route entered native image pool")
	}
	model, ok := resolveNativeModel("bfl/flux-2-pro", WorkloadImageGeneration)
	if !ok || model.ProviderID != AccountTypeBFL || len(model.OutputModalities) != 1 || model.OutputModalities[0] != "image" {
		t.Fatalf("native descriptor=%+v ok=%v", model, ok)
	}
}

func TestBFLNativeImageGenerationPoolsAndAccountsExactlyOnce(t *testing.T) {
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-key") != "bfl-secret" && r.URL.Path != "/image.jpg" {
			t.Fatalf("missing BFL authentication")
		}
		switch r.URL.Path {
		case "/v1/flux-2-pro":
			if r.Method != http.MethodPost {
				t.Fatalf("submit method=%s", r.Method)
			}
			io.WriteString(w, `{"id":"job-1","polling_url":"`+server.URL+`/poll/job-1"}`)
		case "/poll/job-1":
			io.WriteString(w, `{"status":"Ready","result":{"sample":"`+server.URL+`/image.jpg"}}`)
		case "/image.jpg":
			w.Header().Set("Content-Type", "image/png")
			w.Write(image)
		case "/v1/credits":
			io.WriteString(w, `{"credits":9}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.Close()
	connection := &ProviderConnection{Type: AccountTypeBFL, ID: "bfl-one", AccessToken: "bfl-secret", PlanType: "credits"}
	registry := NewProviderRegistry(NewCodexProvider(base, base, base), NewClaudeProvider(base), NewGeminiProvider(base, base), NewBFLProvider(base))
	h := &proxyHandler{cfg: &config{requestTimeout: 5 * time.Second}, transport: http.DefaultTransport, pool: newProviderPool([]*ProviderConnection{connection}), registry: registry, connections: NewConnectionSelector(newProviderPool(nil)), analyticsStore: analytics, metrics: newMetrics()}
	h.connections = NewConnectionSelector(h.pool)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{"model":"bfl/flux-2-pro","prompt":"blue circle","n":1}`))
	if !h.handleNativeImageGeneration(recorder, request, []byte(`{"model":"bfl/flux-2-pro","prompt":"blue circle","n":1}`), "user", "origin", "request-one") {
		t.Fatal("native handler declined BFL model")
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), base64.StdEncoding.EncodeToString(image)) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if h.metrics.requests["image_success"] != 1 {
		t.Fatalf("image success metrics=%v", h.metrics.requests)
	}
	connection.mu.Lock()
	creditsKnown, creditsBalance := connection.Usage.HasCredits, connection.Usage.CreditsBalance
	connection.mu.Unlock()
	if !creditsKnown || creditsBalance != 9 {
		t.Fatalf("credits known=%v balance=%v", creditsKnown, creditsBalance)
	}
	var count, imageCount, economicsKnown, mediaCostIsNull int
	var workload, status, mime, operationID string
	var width, height int
	if err := analytics.db.QueryRow(`SELECT COUNT(*), workload_kind, status, image_count, image_mime, economics_known, media_cost_usd IS NULL, operation_id, image_width, image_height FROM usage_events WHERE request_id='request-one' AND provider_id='bfl' AND model_id='bfl/flux-2-pro'`).Scan(&count, &workload, &status, &imageCount, &mime, &economicsKnown, &mediaCostIsNull, &operationID, &width, &height); err != nil || count != 1 || workload != "image_generation" || status != "success" || imageCount != 1 || mime != "image/png" || economicsKnown != 0 || mediaCostIsNull != 1 || operationID != "job-1" || width != 1 || height != 1 {
		t.Fatalf("usage count=%d workload=%q status=%q images=%d mime=%q economics=%d media_cost_null=%d operation=%q size=%dx%d err=%v", count, workload, status, imageCount, mime, economicsKnown, mediaCostIsNull, operationID, width, height, err)
	}
}

func TestUnknownBFLModelFailsWithoutLegacyFallback(t *testing.T) {
	h := &proxyHandler{}
	body := []byte(`{"model":"bfl/not-real","prompt":"x"}`)
	recorder := httptest.NewRecorder()
	if !h.handleNativeImageGeneration(recorder, httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body)), body, "", "", "unknown") || recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBFLRequestValidationDoesNotCallUpstream(t *testing.T) {
	calls := 0
	base, _ := url.Parse("https://api.bfl.ai")
	connection := &ProviderConnection{Type: AccountTypeBFL, ID: "bfl", AccessToken: "secret"}
	h := &proxyHandler{cfg: &config{}, transport: roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected") }), pool: newProviderPool([]*ProviderConnection{connection}), registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewBFLProvider(base))}
	h.connections = NewConnectionSelector(h.pool)
	for _, test := range []struct{ method, body string }{
		{http.MethodGet, `{"model":"bfl/flux-2-pro","prompt":"x"}`},
		{http.MethodPost, `{"model":"bfl/flux-2-pro","prompt":""}`},
		{http.MethodPost, `{"model":"bfl/flux-2-pro","prompt":"x","n":2}`},
		{http.MethodPost, `{"model":"bfl/flux-2-pro","prompt":"x","response_format":"binary"}`},
		{http.MethodPost, `{"model":"bfl/flux-2-pro","prompt":"x","size":"bad"}`},
	} {
		recorder := httptest.NewRecorder()
		h.handleNativeImageGeneration(recorder, httptest.NewRequest(test.method, "/v1/images/generations", strings.NewReader(test.body)), []byte(test.body), "", "", "validation")
		if recorder.Code < 400 {
			t.Fatalf("method=%s body=%s status=%d", test.method, test.body, recorder.Code)
		}
	}
	if calls != 0 {
		t.Fatalf("validation made %d upstream calls", calls)
	}
}

func TestBFLPollingHonorsCancellation(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			io.WriteString(w, `{"id":"job","polling_url":"`+server.URL+`/poll"}`)
			return
		}
		io.WriteString(w, `{"status":"Pending"}`)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err := executeBFLImage(ctx, http.DefaultTransport, NewBFLProvider(base), &ProviderConnection{Type: AccountTypeBFL, AccessToken: "key"}, nativeImageModels[0], nativeImageRequest{Prompt: "cancel"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v", err)
	}
}

func TestBFLZeroCreditsConnectionIsExcluded(t *testing.T) {
	zero := &ProviderConnection{Type: AccountTypeBFL, ID: "zero", Usage: UsageSnapshot{HasCredits: true, CreditsBalance: 0}}
	unknown := &ProviderConnection{Type: AccountTypeBFL, ID: "unknown"}
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{zero, unknown}))
	if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeBFL}); got != unknown {
		t.Fatalf("selected=%v, want unknown-balance connection", got)
	}
}

func TestBoundedBodyRejectsOverflow(t *testing.T) {
	if _, err := readBoundedBody(strings.NewReader(strings.Repeat("x", 11)), 10, "test body"); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("error=%v", err)
	}
}

func TestBFLMultiAccountSelectionSkipsCoolingConnection(t *testing.T) {
	cooling := &ProviderConnection{Type: AccountTypeBFL, ID: "cooling", AccessToken: "first", RateLimitUntil: time.Now().Add(time.Hour)}
	healthy := &ProviderConnection{Type: AccountTypeBFL, ID: "healthy", AccessToken: "second"}
	selector := NewConnectionSelector(newProviderPool([]*ProviderConnection{cooling, healthy}))
	if got := selector.Select(ConnectionSelection{ProviderID: AccountTypeBFL, Model: "flux-2-pro"}); got != healthy {
		t.Fatalf("selected=%v, want healthy", got)
	}
}

func TestBFLFailureIsAccountedExactlyOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider failure", http.StatusInternalServerError)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.Close()
	connection := &ProviderConnection{Type: AccountTypeBFL, ID: "failed", AccessToken: "key"}
	h := &proxyHandler{cfg: &config{}, transport: http.DefaultTransport, pool: newProviderPool([]*ProviderConnection{connection}), registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewBFLProvider(base)), analyticsStore: analytics}
	h.connections = NewConnectionSelector(h.pool)
	body := []byte(`{"model":"bfl/flux-2-pro","prompt":"failure"}`)
	recorder := httptest.NewRecorder()
	h.handleNativeImageGeneration(recorder, httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body)), body, "user", "origin", "failure-request")
	var count int
	var status, workload string
	if err := analytics.db.QueryRow(`SELECT COUNT(*), status, workload_kind FROM usage_events WHERE request_id='failure-request'`).Scan(&count, &status, &workload); err != nil || count != 1 || status != "failed" || workload != "image_generation" {
		t.Fatalf("count=%d status=%q workload=%q err=%v", count, status, workload, err)
	}
}

func TestBFLPaymentRequiredPersistsZeroCapacity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bfl.json")
	if err := os.WriteFile(path, []byte(`{"api_key":"key","credits_balance":5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	connection := &ProviderConnection{Type: AccountTypeBFL, ID: "bfl", File: path, AccessToken: "key", Usage: UsageSnapshot{HasCredits: true, CreditsBalance: 5}}
	h := &proxyHandler{}
	h.applyNativeImageFailure(connection, &nativeImageUpstreamError{StatusCode: http.StatusPaymentRequired})
	connection.mu.Lock()
	balance := connection.Usage.CreditsBalance
	connection.mu.Unlock()
	if balance != 0 {
		t.Fatalf("balance=%v", balance)
	}
	reloaded, err := NewBFLProvider(nil).LoadAccount("bfl.json", path, mustReadFile(t, path))
	if err != nil || reloaded == nil || !reloaded.Usage.HasCredits || reloaded.Usage.CreditsBalance != 0 {
		t.Fatalf("reloaded=%+v err=%v", reloaded, err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestBFLRateLimitCoolsConnectionWithoutRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "60")
		http.Error(w, "limited", http.StatusTooManyRequests)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	connection := &ProviderConnection{Type: AccountTypeBFL, ID: "limited", AccessToken: "secret"}
	h := &proxyHandler{cfg: &config{}, transport: http.DefaultTransport, pool: newProviderPool([]*ProviderConnection{connection}), registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewBFLProvider(base))}
	h.connections = NewConnectionSelector(h.pool)
	recorder := httptest.NewRecorder()
	body := []byte(`{"model":"bfl/flux-2-pro","prompt":"one request"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	if !h.handleNativeImageGeneration(recorder, request, body, "user", "origin", "limited-request") {
		t.Fatal("handler declined native model")
	}
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "60" || calls != 1 {
		t.Fatalf("status=%d retry=%q calls=%d", recorder.Code, recorder.Header().Get("Retry-After"), calls)
	}
	connection.mu.Lock()
	cooling := connection.RateLimitUntil.After(time.Now().Add(50 * time.Second))
	connection.mu.Unlock()
	if !cooling || h.connectionSelector().Select(ConnectionSelection{ProviderID: AccountTypeBFL}) != nil {
		t.Fatal("rate-limited BFL connection remained routable")
	}
}

func TestBFLRejectsMalformedImageBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		io.WriteString(w, "not an image")
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	_, err := downloadBFLImage(t.Context(), http.DefaultTransport, base, server.URL)
	if err == nil || !strings.Contains(err.Error(), "invalid image bytes") {
		t.Fatalf("error=%v", err)
	}
}

func TestBFLRejectsUntrustedPollingURL(t *testing.T) {
	for _, raw := range []string{"https://attacker.example/poll", "https://api.bfl.ai.evil.example/poll", "https://api.bfl.ai:444/poll", "https://api.bfl.ai/poll#fragment"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"id":"job","polling_url":"`+raw+`"}`)
		}))
		base, _ := url.Parse(server.URL)
		connection := &ProviderConnection{Type: AccountTypeBFL, ID: "bfl", AccessToken: "secret"}
		_, err := executeBFLImage(t.Context(), http.DefaultTransport, NewBFLProvider(base), connection, nativeImageModels[0], nativeImageRequest{Prompt: "safe"})
		server.Close()
		if err == nil || !strings.Contains(err.Error(), "invalid polling_url") {
			t.Fatalf("url=%s error=%v", raw, err)
		}
	}
}

func TestBFLAdminValidatesAndStoresKeySeparately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/credits" || r.Header.Get("x-key") != "bfl-key" {
			t.Fatalf("validation path=%s key=%q", r.URL.Path, r.Header.Get("x-key"))
		}
		io.WriteString(w, `{"credits":10}`)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	dir := t.TempDir()
	h := &proxyHandler{cfg: &config{poolDir: dir, bflBase: base}, transport: http.DefaultTransport, pool: newProviderPool(nil), registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewBFLProvider(base))}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/admin/bfl/add", strings.NewReader(`{"api_key":"bfl-key"}`))
	request.Header.Set("Content-Type", "application/json")
	h.handleBFLAdd(recorder, request)
	if recorder.Code != http.StatusOK || h.pool.countByType(AccountTypeBFL) != 1 {
		t.Fatalf("status=%d body=%s count=%d", recorder.Code, recorder.Body.String(), h.pool.countByType(AccountTypeBFL))
	}
	stored := h.pool.allAccounts()[0]
	stored.mu.Lock()
	hasCredits, balance := stored.Usage.HasCredits, stored.Usage.CreditsBalance
	stored.mu.Unlock()
	if !hasCredits || balance != 10 {
		t.Fatalf("credits known=%v balance=%v", hasCredits, balance)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "bfl"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("BFL files=%d err=%v", len(entries), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "codex")); !os.IsNotExist(err) {
		t.Fatal("BFL contribution touched Codex credential storage")
	}
}

func TestBFLAdminRejectsInvalidCreditsResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"credits":-1}`, `not-json`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			dir := t.TempDir()
			h := &proxyHandler{cfg: &config{poolDir: dir, bflBase: base}, transport: http.DefaultTransport, pool: newProviderPool(nil)}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/admin/bfl/add", strings.NewReader(`{"api_key":"fake-bfl-key"}`))
			request.Header.Set("Content-Type", "application/json")
			h.handleBFLAdd(recorder, request)
			if recorder.Code != http.StatusBadGateway || h.pool.countByType(AccountTypeBFL) != 0 {
				t.Fatalf("status=%d body=%s count=%d", recorder.Code, recorder.Body.String(), h.pool.countByType(AccountTypeBFL))
			}
		})
	}
}

func TestBFLContributionReusesExistingConnectionForSameKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"credits":10}`) }))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(path, []byte(`{"api_key":"bfl-key","dead":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	connection := &ProviderConnection{Type: AccountTypeBFL, ID: "stable", File: path, AccessToken: "bfl-key", Dead: true}
	h := &proxyHandler{cfg: &config{poolDir: dir, bflBase: base}, transport: http.DefaultTransport, pool: newProviderPool([]*ProviderConnection{connection}), registry: NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewBFLProvider(base))}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/admin/bfl/add", strings.NewReader(`{"api_key":"bfl-key"}`))
	request.Header.Set("Content-Type", "application/json")
	h.handleBFLAdd(recorder, request)
	if recorder.Code != http.StatusOK || h.pool.countByType(AccountTypeBFL) != 1 || !strings.Contains(recorder.Body.String(), `"existing":true`) || !strings.Contains(recorder.Body.String(), `"account_id":"stable"`) {
		t.Fatalf("status=%d body=%s count=%d", recorder.Code, recorder.Body.String(), h.pool.countByType(AccountTypeBFL))
	}
	connection.mu.Lock()
	dead := connection.Dead
	connection.mu.Unlock()
	if dead {
		t.Fatal("validated existing BFL connection remained dead")
	}
}

func TestBFLModelRoutingProjectionUsesNativeWorkloadRoute(t *testing.T) {
	base, _ := url.Parse("https://api.bfl.ai")
	ready := &ProviderConnection{Type: AccountTypeBFL, ID: "ready", Identity: ConnectionIdentity{DisplayName: "Ready"}}
	cooling := &ProviderConnection{Type: AccountTypeBFL, ID: "cooling", Identity: ConnectionIdentity{DisplayName: "Cooling"}, RateLimitUntil: time.Now().Add(time.Hour)}
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewBFLProvider(base))
	h := &proxyHandler{pool: newProviderPool([]*ProviderConnection{ready, cooling}), registry: registry, modelRoutes: NewModelRouteRegistry(registry)}
	recorder := httptest.NewRecorder()
	h.serveModelRouting(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/models/bfl%2Fflux-2-pro/routing", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var projection ModelRoutingProjection
	if err := json.Unmarshal(recorder.Body.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	if projection.ProviderID != AccountTypeBFL || projection.CanonicalModel != "bfl/flux-2-pro" || len(projection.EligibleConnections) != 1 || len(projection.ExcludedConnections) != 1 || projection.ExcludedConnections[0].Reason != "rate-limit cooldown" {
		t.Fatalf("projection=%+v", projection)
	}
}

func TestBFLAccountReloadPreservesStableIdentityAndLifecycle(t *testing.T) {
	dir := t.TempDir()
	providerDir := filepath.Join(dir, "bfl")
	if err := os.MkdirAll(providerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(providerDir, "stable.json")
	if err := os.WriteFile(path, []byte(`{"type":"bfl","api_key":"secret","disabled":true,"plan_type":"credits","identity":{"display_name":"BFL account"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := NewProviderRegistry(&CodexProvider{}, &ClaudeProvider{}, &GeminiProvider{}, NewBFLProvider(nil))
	accounts, err := loadPool(dir, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].ID != "stable" || !accounts[0].Disabled || accounts[0].Type != AccountTypeBFL {
		t.Fatalf("accounts=%+v", accounts)
	}
	if err := saveAccount(accounts[0]); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadPool(dir, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded) != 1 || reloaded[0].ID != "stable" || !reloaded[0].Disabled || reloaded[0].AccessToken != "secret" {
		t.Fatalf("reloaded=%+v", reloaded)
	}
}

func TestPoolCatalogSeparatesNativeImageOutputFromVisionInput(t *testing.T) {
	var native *poolModelDescriptor
	descriptors := poolModelDescriptors(newProviderPool([]*ProviderConnection{{Type: AccountTypeBFL, ID: "bfl"}}))
	for index := range descriptors {
		descriptor := descriptors[index]
		if descriptor.ID == "bfl/flux-2-pro" {
			native = &descriptor
			break
		}
	}
	if native == nil || native.ModelKind != WorkloadImageGeneration || !native.Capabilities["native_image_generation"] || len(native.OutputModalities) != 1 || native.OutputModalities[0] != "image" {
		t.Fatalf("native catalog descriptor=%+v", native)
	}
}
