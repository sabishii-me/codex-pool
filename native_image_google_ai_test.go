package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func googleAIImagePNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestGoogleAIImageIsSeparateNativePool(t *testing.T) {
	model, ok := resolveNativeModel("google-ai-image/gemini-2.5-flash-image", WorkloadImageGeneration)
	if !ok || model.ProviderID != AccountTypeGoogleAIImage {
		t.Fatalf("model=%+v ok=%v", model, ok)
	}
	if _, ok := resolveNativeModel("gemini/gemini-2.5-flash-image", WorkloadImageGeneration); ok {
		t.Fatal("Gemini LLM entered native image pool")
	}
	if _, ok := resolveNativeModel("antigravity/gemini-3.1-flash-image", WorkloadImageGeneration); ok {
		t.Fatal("Antigravity entered native image pool")
	}
}

func TestGoogleAIImageGenerationUsesAIStudioKeyAndAccounts(t *testing.T) {
	imageBytes := googleAIImagePNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash-image:generateContent" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "studio-key" || r.Header.Get("Authorization") != "" {
			t.Fatalf("headers=%v", r.Header)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"responseModalities":["IMAGE"]`) {
			t.Fatalf("body=%s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"responseId":"operation-one","candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"`+base64.StdEncoding.EncodeToString(imageBytes)+`"}}]}}]}`)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	connection := &ProviderConnection{Type: AccountTypeGoogleAIImage, ID: "studio-one", AccessToken: "studio-key", PlanType: "ai-studio"}
	analytics, err := newAnalyticsStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer analytics.Close()
	pool := newProviderPool([]*ProviderConnection{connection})
	h := &proxyHandler{cfg: &config{requestTimeout: 5 * time.Second}, transport: http.DefaultTransport, pool: pool, registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewGoogleAIImageProvider(base)), connections: NewConnectionSelector(pool), analyticsStore: analytics}
	body := []byte(`{"model":"google-ai-image/gemini-2.5-flash-image","prompt":"red point"}`)
	recorder := httptest.NewRecorder()
	if !h.handleNativeImageGeneration(recorder, httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body)), body, "user", "origin", "request") {
		t.Fatal("handler declined model")
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), base64.StdEncoding.EncodeToString(imageBytes)) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGoogleAIImageContributionValidatesAndStoresSeparateType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" || r.Header.Get("x-goog-api-key") != "studio-key" {
			t.Fatalf("path=%s headers=%v", r.URL.Path, r.Header)
		}
		io.WriteString(w, `{"models":[]}`)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	dir := t.TempDir()
	pool := newProviderPool(nil)
	h := &proxyHandler{cfg: &config{poolDir: dir, googleAIImageBase: base}, transport: http.DefaultTransport, pool: pool, registry: NewProviderRegistry(&CodexProvider{}, &GeminiProvider{}, NewGoogleAIImageProvider(base))}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/pool/accounts/google-ai-image/add", strings.NewReader(`{"api_key":"studio-key"}`))
	request.Header.Set("Content-Type", "application/json")
	h.handleGoogleAIImageAdd(recorder, request)
	if recorder.Code != http.StatusOK || h.pool.countByType(AccountTypeGoogleAIImage) != 1 || h.pool.countByType(AccountTypeGemini) != 0 || h.pool.countByType(AccountTypeAntigravity) != 0 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
