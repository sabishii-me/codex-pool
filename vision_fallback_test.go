package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func visionTestProviderSpec() ProviderSpec {
	return ProviderSpec{
		ID: "visionprovider", Protocol: ProtocolOpenAIChat, BaseURL: "https://visionprovider.example.test/v1",
		PlanType: "vision", CredentialField: "api_key", Auth: ProviderAuthSpec{Type: AuthBearer},
		Models: []ModelRouteSpec{
			{ID: "vision-model", Input: []string{"text", "image"}},
			{ID: "text-only-model", Input: []string{"text"}, VisionFallback: "vision-model"},
		},
	}
}

func TestRequestBodyHasImage(t *testing.T) {
	imageBody := []byte(`{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`)
	if !requestBodyHasImage(imageBody) {
		t.Fatal("chat image_url not detected")
	}
	responsesBody := []byte(`{"model":"m","input":[{"role":"user","content":[{"type":"input_text","text":"hi"},{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}]}`)
	if !requestBodyHasImage(responsesBody) {
		t.Fatal("responses input_image not detected")
	}
	claudeBody := []byte(`{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}]}`)
	if !requestBodyHasImage(claudeBody) {
		t.Fatal("claude image block not detected")
	}
	textBody := []byte(`{"model":"m","messages":[{"role":"user","content":"plain text"}]}`)
	if requestBodyHasImage(textBody) {
		t.Fatal("plain text falsely detected as image")
	}
}

func TestVisionFallbackModel(t *testing.T) {
	provider, err := NewDeclarativeProvider(visionTestProviderSpec())
	if err != nil {
		t.Fatal(err)
	}
	base, _ := url.Parse("http://127.0.0.1:1")
	registry := NewProviderRegistry(NewCodexProvider(base, base, base), NewGeminiProvider(base, base), provider)
	if got := visionFallbackModel(registry, "text-only-model"); got != "vision-model" {
		t.Fatalf("text-only fallback=%q want vision-model", got)
	}
	if got := visionFallbackModel(registry, "vision-model"); got != "" {
		t.Fatalf("vision model should have no fallback, got %q", got)
	}
	if !modelSupportsImage(registry, "vision-model") {
		t.Fatal("vision-model should support image")
	}
	if modelSupportsImage(registry, "text-only-model") {
		t.Fatal("text-only-model should not support image")
	}
}

// TestVisionFallbackRoutesImageRequestToVisionModel verifies an image request
// to a non-vision declarative model is transparently routed to its configured
// vision fallback: the upstream sees the vision model, and the client sees its
// original model name in the response.
func TestVisionFallbackRoutesImageRequestToVisionModel(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	var upstreamModel string
	var upstreamBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamBody, _ = io.ReadAll(r.Body)
		var obj map[string]any
		if json.Unmarshal(upstreamBody, &obj) == nil {
			upstreamModel, _ = obj["model"].(string)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","model":"vision-model","choices":[{"index":0,"message":{"role":"assistant","content":"saw the image"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3}}`))
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	provider, err := NewDeclarativeProvider(visionTestProviderSpec())
	if err != nil {
		t.Fatal(err)
	}
	provider.baseURL = base
	connection := &ProviderConnection{Type: "visionprovider", ID: "vision-1", AccessToken: "key", PlanType: "vision"}
	handler := &proxyHandler{
		cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
		transport: http.DefaultTransport,
		pool:      newProviderPool([]*ProviderConnection{connection}),
		registry:  NewProviderRegistry(NewCodexProvider(base, base, base), NewGeminiProvider(base, base), provider),
		metrics:   newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
	}
	body := `{"model":"text-only-model","messages":[{"role":"user","content":[{"type":"text","text":"what is this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "vision-fallback-test")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if upstreamModel != "vision-model" {
		t.Fatalf("upstream saw model=%q want vision-model (body=%s)", upstreamModel, upstreamBody)
	}
	var out map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if got, _ := out["model"].(string); got != "text-only-model" {
		t.Fatalf("client response model=%q want text-only-model (transparent)", got)
	}
}

// TestVisionFallbackDoesNotRewriteTextRequest verifies plain-text requests to a
// non-vision model are NOT redirected to the vision fallback.
func TestVisionFallbackDoesNotRewriteTextRequest(t *testing.T) {
	t.Setenv("POOL_JWT_SECRET", "contract-secret")
	var upstreamModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var obj map[string]any
		if json.Unmarshal(body, &obj) == nil {
			upstreamModel, _ = obj["model"].(string)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","model":"text-only-model","choices":[{"index":0,"message":{"role":"assistant","content":"plain"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`))
	}))
	defer upstream.Close()
	base, _ := url.Parse(upstream.URL)
	provider, err := NewDeclarativeProvider(visionTestProviderSpec())
	if err != nil {
		t.Fatal(err)
	}
	provider.baseURL = base
	connection := &ProviderConnection{Type: "visionprovider", ID: "vision-1", AccessToken: "key", PlanType: "vision"}
	handler := &proxyHandler{
		cfg:       &config{requestTimeout: 10 * time.Second, streamTimeout: 10 * time.Second, maxInMemoryBodyBytes: 1024 * 1024},
		transport: http.DefaultTransport,
		pool:      newProviderPool([]*ProviderConnection{connection}),
		registry:  NewProviderRegistry(NewCodexProvider(base, base, base), NewGeminiProvider(base, base), provider),
		metrics:   newMetrics(), recent: newRecentErrors(5), aliases: newModelAliases(nil),
	}
	body := `{"model":"text-only-model","messages":[{"role":"user","content":"plain text"}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+generateClaudePoolToken("contract-secret", "contract-user"))
	response := httptest.NewRecorder()
	handler.proxyRequest(response, request, "vision-no-rewrite-test")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if upstreamModel != "text-only-model" {
		t.Fatalf("upstream saw model=%q want text-only-model (no rewrite for text)", upstreamModel)
	}
}

// TestRequestBodyHasImageNonJSON ensures malformed bodies are handled safely.
func TestRequestBodyHasImageNonJSON(t *testing.T) {
	if requestBodyHasImage([]byte("not json")) {
		t.Fatal("non-JSON body falsely detected as image")
	}
	if requestBodyHasImage(nil) {
		t.Fatal("empty body detected as image")
	}
	if !strings.Contains("x", "x") {
		t.Fatal("unreachable")
	}
}

// TestVisionTransparentWriterSSE verifies SSE responses have their data-line
// model field rewritten while preserving event framing.
func TestVisionTransparentWriterSSE(t *testing.T) {
	var out bytes.Buffer
	vt := newVisionTransparentWriter(&out, "text-only-model")
	vt.sse = true
	event := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"model\":\"vision-model\",\"delta\":\"hi\"}\n\n"
	if _, err := vt.Write([]byte(event)); err != nil {
		t.Fatal(err)
	}
	vt.Flush()
	if strings.Contains(out.String(), "vision-model") {
		t.Fatalf("SSE still contains vision-model: %q", out.String())
	}
	if !strings.Contains(out.String(), `"model":"text-only-model"`) {
		t.Fatalf("SSE missing rewritten model: %q", out.String())
	}
	if !strings.Contains(out.String(), "event: response.output_text.delta") {
		t.Fatalf("SSE event framing lost: %q", out.String())
	}
}

// TestVisionTransparentWriterJSON verifies a non-streaming JSON response gets
// its top-level model field rewritten.
func TestVisionTransparentWriterJSON(t *testing.T) {
	var out bytes.Buffer
	vt := newVisionTransparentWriter(&out, "text-only-model")
	body := `{"id":"chatcmpl-1","object":"chat.completion","model":"vision-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`
	if _, err := vt.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	vt.Flush()
	if strings.Contains(out.String(), "vision-model") {
		t.Fatalf("JSON still contains vision-model: %q", out.String())
	}
	if !strings.Contains(out.String(), `"model":"text-only-model"`) {
		t.Fatalf("JSON missing rewritten model: %q", out.String())
	}
	// Ensure response is still valid JSON.
	var obj map[string]any
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("rewritten JSON invalid: %v body=%s", err, out.String())
	}
}
