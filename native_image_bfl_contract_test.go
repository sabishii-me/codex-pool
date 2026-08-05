package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBFLTerminalPollingStatusesDoNotHang(t *testing.T) {
	for _, test := range []struct{ status, class string }{
		{"Request Moderated", "moderation"}, {"Content Moderated", "moderation"},
		{"Task not found", "operation_not_found"}, {"Error", "provider_error"},
	} {
		t.Run(test.status, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					io.WriteString(w, `{"id":"job","polling_url":"`+server.URL+`/poll"}`)
					return
				}
				io.WriteString(w, `{"id":"job","status":"`+test.status+`","result":null}`)
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			_, err := executeBFLImage(ctx, http.DefaultTransport, NewBFLProvider(base), &ProviderConnection{Type: AccountTypeBFL, AccessToken: "key"}, nativeImageModels[0], nativeImageRequest{Prompt: "test"})
			if err == nil || nativeImageFailureClass(err) != test.class {
				t.Fatalf("status=%q error=%v class=%q", test.status, err, nativeImageFailureClass(err))
			}
		})
	}
}

func TestNativeImageErrorStatusPreservesTypedProviderSemantics(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{&nativeImageJobError{Status: "Request Moderated"}, http.StatusUnprocessableEntity},
		{&nativeImageUpstreamError{StatusCode: http.StatusPaymentRequired}, http.StatusPaymentRequired},
		{&nativeImageUpstreamError{StatusCode: http.StatusUnauthorized}, http.StatusUnauthorized},
		{&nativeImageUpstreamError{StatusCode: http.StatusUnprocessableEntity}, http.StatusUnprocessableEntity},
		{&nativeImageUpstreamError{StatusCode: http.StatusServiceUnavailable}, http.StatusBadGateway},
	} {
		if got := nativeImageErrorStatus(test.err); got != test.status {
			t.Fatalf("error=%v status=%d want=%d", test.err, got, test.status)
		}
	}
}

func TestBFLRequestForcesDocumentedPNGOutput(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		payload, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(payload), `"output_format":"png"`) {
			t.Fatalf("submit payload=%s", payload)
		}
		return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("failure"))}, nil
	})
	base, _ := url.Parse("https://api.bfl.ai")
	_, _ = executeBFLImage(t.Context(), transport, NewBFLProvider(base), &ProviderConnection{Type: AccountTypeBFL, AccessToken: "key"}, nativeImageModels[0], nativeImageRequest{Prompt: "test"})
}
