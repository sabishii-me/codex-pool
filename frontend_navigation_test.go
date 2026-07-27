package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrontendNavigationBoundary(t *testing.T) {
	canonical := []string{"/", "/models", "/usage", "/setup", "/profile", "/admin/connections", "/admin/members", "/admin/system"}
	for _, path := range canonical {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Accept", "text/html,application/xhtml+xml")
			if !isFrontendNavigationRequest(req) {
				t.Fatalf("browser navigation %s was not admitted to SPA", path)
			}
		})
	}
}

func TestFrontendNavigationBoundaryRejectsAPIsProxyTrafficAndRemovedUIRoutes(t *testing.T) {
	paths := []string{"/api/pool/session", "/api/pool/stats", "/auth/login/google", "/config/pi/token", "/setup/pi/token", "/v1/responses", "/backend-api/wham/usage", "/healthz", "/assets/index.js", "/friend/code", "/operator", "/operator/monitor", "/admin/routes", "/admin/usage", "/admin/monitor", "/unknown"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "text/html")
		if isFrontendNavigationRequest(req) {
			t.Fatalf("API/proxy path %s was incorrectly admitted to SPA", path)
		}
	}
}

func TestRemovedFrontendPathsAreTombstoned(t *testing.T) {
	h := &proxyHandler{cfg: &config{}}
	for _, path := range []string{"/friend", "/friend/code", "/operator", "/operator/monitor", "/admin/routes", "/admin/usage", "/admin/monitor", "/hero.png", "/hero.webp", "/og-image.png"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Accept", "text/html")
		h.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d body=%q", path, recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("%s returned an HTML compatibility surface", path)
		}
	}
}

func TestFrontendNavigationBoundaryRequiresHTMLIntent(t *testing.T) {
	for _, accept := range []string{"", "application/json", "text/event-stream", "*/*"} {
		req := httptest.NewRequest(http.MethodGet, "/usage", nil)
		req.Header.Set("Accept", accept)
		if isFrontendNavigationRequest(req) {
			t.Fatalf("Accept %q incorrectly admitted to SPA", accept)
		}
	}
}

func TestFrontendNavigationBoundaryServesEmbeddedShell(t *testing.T) {
	h := &proxyHandler{cfg: &config{oauthGoogleClientID: "configured"}}
	for _, path := range []string{"/usage", "/models", "/admin/system"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "text/html")
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%q", path, recorder.Code, recorder.Body.String())
		}
		if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
			t.Fatalf("%s content-type=%q", path, contentType)
		}
		if !strings.Contains(recorder.Body.String(), `<div id="root"></div>`) {
			t.Fatalf("%s did not serve embedded React shell", path)
		}
	}
}
