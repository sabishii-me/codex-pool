package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
