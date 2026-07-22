package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCodexOAuthRelayForwardsOnlyCallbackFieldsAndFinishes(t *testing.T) {
	gateway, err := url.Parse("http://127.0.0.1:18990/base")
	if err != nil {
		t.Fatal(err)
	}
	var finished atomic.Int32
	handler := newCodexOAuthRelayHandler(*gateway, func() { finished.Add(1) })
	request := httptest.NewRequest(http.MethodGet, "/auth/callback?code=secret-code&state=random-state&iss=https%3A%2F%2Fauth.openai.com&ignored=value", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.String() != "http://127.0.0.1:18990/base/auth/callback/codex?code=secret-code&iss=https%3A%2F%2Fauth.openai.com&state=random-state" {
		t.Fatalf("location = %q", location.String())
	}
	if finished.Load() != 1 {
		t.Fatalf("finish calls = %d", finished.Load())
	}
}

func TestCodexOAuthRelayRejectsIncompleteCallbackWithoutFinishing(t *testing.T) {
	gateway, _ := url.Parse("http://127.0.0.1:18990")
	var finished atomic.Int32
	handler := newCodexOAuthRelayHandler(*gateway, func() { finished.Add(1) })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/auth/callback?state=state-only", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid Codex OAuth callback") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if finished.Load() != 0 {
		t.Fatalf("finish calls = %d", finished.Load())
	}
}
